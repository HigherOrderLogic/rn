// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package extension

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/component/asciiart/capture"
)

const defaultFPS = 30

var (
	cmdGetUserMedia = textapi.CommandManual{
		Name: "rtcgetusermedia",
		Summary: "Opens up a new window with an ASCII-encoded feed of the user's default" +
			" video input device. This is a prototype that will be evolved into WebRTC " +
			"peer-to-peer calling system.",
	}
	cmdConvertImage = textapi.CommandManual{
		Name: "rtcconvertimage",
		Summary: "Converts a local JPEG image to an 130x70 ASCII encoded image and opens" +
			" up a window to display it.",
		Synopsis: "image",
	}
)

// NewExtension returns the RTC extension and its metadata.
func NewExtension() (extensionapi.WorkspaceExtension, extensionapi.Metadata) {
	return workspaceExtension{}, extensionapi.Metadata{
		DeveloperID:      "Unstable Build",
		DeveloperEmail:   "it@unstable.build",
		DeveloperKey:     "064D4ABCFA6D9338",
		ExtensionID:      "rtc",
		ExtensionName:    "RTC",
		ExtensionVersion: "development",
		Permissions: extensionapi.NewPermissions(
			extensionapi.PermissionCommands,
			extensionapi.PermissionBrowserWindowManager,
			extensionapi.PermissionInterrupt,
		),
	}
}

type workspaceExtension struct{}

func (workspaceExtension) ExtendWorkspace(
	ctx context.Context, w *extensionapi.Workspace, cfg config.Config,
) error {
	h := &rtcCommandHandler{
		wm:          w.WindowManager(ctx),
		interrupter: w.Interrupter(ctx),
		cfg:         cfg,
	}
	if err := w.RegisterCommand(cmdGetUserMedia, h); err != nil {
		return fmt.Errorf("register command %q: %w", cmdGetUserMedia.Name, err)
	}
	if err := w.RegisterCommand(cmdConvertImage, h); err != nil {
		return fmt.Errorf("register command %q: %w", cmdConvertImage.Name, err)
	}
	return nil
}

type rtcCommandHandler struct {
	mu          sync.Mutex
	wm          browserapi.WindowManager
	interrupter term.Interrupter
	cfg         config.Config
	windows     map[string]browserapi.Window
}

func (h *rtcCommandHandler) HandleCommand(ctx context.Context, cmd textapi.Command) error {
	h.mu.Lock()
	if h.windows == nil {
		h.windows = make(map[string]browserapi.Window)
	}
	if h.windows[cmd.Name] != nil {
		h.mu.Unlock()
		return nil
	}
	h.mu.Unlock()

	handler, err := h.newHandler(ctx, cmd)
	if err != nil {
		return err
	}
	cleaningHandler := browserapi.FuncHandler(handler, func() error {
		h.mu.Lock()
		delete(h.windows, cmd.Name)
		h.mu.Unlock()
		return handler.Close()
	})
	win, err := h.wm.Split(browserapi.OrientationRight, cmd.Window, cleaningHandler)
	if err != nil {
		return fmt.Errorf("split: %w", err)
	}

	h.mu.Lock()
	h.windows[cmd.Name] = win
	h.mu.Unlock()
	return nil
}

func (h *rtcCommandHandler) Complete(
	ctx context.Context, cmd string, args []string,
) (iterator.Iterator[string], error) {
	return iterator.FromSlice[string](nil), nil
}

func (h *rtcCommandHandler) newHandler(ctx context.Context, cmd textapi.Command) (browserapi.Handler, error) {
	switch cmd.Name {
	case cmdGetUserMedia.Name:
		return h.newUserMediaHandler(ctx)
	case cmdConvertImage.Name:
		return newConvertImageHandler(cmd)
	default:
		return nil, nil
	}
}

func (h *rtcCommandHandler) newUserMediaHandler(ctx context.Context) (browserapi.Handler, error) {
	if h.interrupter == nil {
		return nil, errors.New("missing event publisher permission")
	}

	imageConfig := asciiart.DefaultConfig()
	if color, err := h.cfg.GetBool("color"); err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'color' from config: %v", err)
		}
	} else {
		imageConfig.Color = color
	}
	if contrast, err := h.cfg.GetInt("contrast"); err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'contrast' from config: %v", err)
		}
	} else {
		imageConfig.AdjustContrast = float64(contrast)
	}
	if maintain, err := h.cfg.GetBool("maintain_aspect_ratio"); err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'maintain_aspect_ratio' from config: %v", err)
		}
	} else {
		imageConfig.MaintainAspectRatio = maintain
	}

	if chars, err := h.cfg.GetString("density_characters"); err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'density_characters' from config: %v", err)
		}
	} else {
		imageConfig.DensityCharacters = chars
	}

	fps, err := h.cfg.GetInt("fps")
	if err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'fps' from config: %v", err)
		}
		fps = defaultFPS
	}
	device, err := capture.NewDevice(h.interrupter, fps, imageConfig)
	if err != nil {
		return nil, fmt.Errorf("new device: %v", err)
	}
	return browserapi.FuncHandler(
		handler.NopFromComponent(component.Sync(new(sync.Mutex), device)), device.Close), nil
}

func newConvertImageHandler(cmd textapi.Command) (browserapi.Handler, error) {
	if len(cmd.Args) < 1 {
		return nil, errors.New("expected first argument to be a JPEG image URI")
	}
	img, err := openImage(cmd.Args[0])
	if err != nil {
		return nil, err
	}
	cfg := asciiart.DefaultConfig()
	cfg.Color = true
	cfg.MaintainAspectRatio = true
	return browserapi.NopHandler(handler.NopFromComponent(
		component.Sync(new(sync.Mutex), asciiart.NewComponent(img, cfg)))), nil
}

func openImage(filename string) (image.Image, error) {
	fl, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	defer fl.Close()
	img, err := jpeg.Decode(fl)
	if err != nil {
		return nil, fmt.Errorf("jpeg decode: %v", err)
	}
	return img, nil
}
