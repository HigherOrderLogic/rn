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
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/api/browserapi/browserext"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/component/asciiart/capture"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/rpc"
)

const defaultFPS = 30

// Grantee returns this extension's extension.Grantee, and it required permissions.
func Grantee() (extension.Grantee, []extensionapi.Permission) {
	webcamGrantee, perms := extutil.NewCommandSplitHandler(extutil.CommandSplitHandlerConfig{
		SplitOrientation: browserapi.OrientationRight,
		Handler: func(ctx context.Context, cmd textapi.Command,
			grants []extension.Grant, broker rpc.MuxBroker,
			invokeWindow browserapi.Window, pconfig config.Config,
		) (extutil.RedispatchHandler, error) {
			var publisher term.Interrupter
			for _, grant := range grants {
				if grant.Permission == extensionapi.PermissionInterrupt {
					var err error
					publisher, err = browserext.EventPublisher(ctx, grant, broker)
					if err != nil {
						return nil, fmt.Errorf("browser extension event publisher: %v", err)
					}
				}
			}

			if publisher == nil {
				return nil, errors.New("missing event publisher permission")
			}

			imageConfig := asciiart.DefaultConfig()
			if color, err := pconfig.GetBool("color"); err != nil {
				if err != config.ErrNotFound {
					log.Warningf("failed to get 'color' from config: %v", err)
				}
			} else {
				imageConfig.Color = color
			}
			if contrast, err := pconfig.GetInt("contrast"); err != nil {
				if err != config.ErrNotFound {
					log.Warningf("failed to get 'contrast' from config: %v", err)
				}
			} else {
				imageConfig.AdjustContrast = float64(contrast)
			}
			if maintain, err := pconfig.GetBool("maintain_aspect_ratio"); err != nil {
				if err != config.ErrNotFound {
					log.Warningf("failed to get 'maintain_aspect_ratio' from config: %v", err)
				}
			} else {
				imageConfig.MaintainAspectRatio = maintain
			}

			if chars, err := pconfig.GetString("density_characters"); err != nil {
				if err != config.ErrNotFound {
					log.Warningf("failed to get 'density_characters' from config: %v", err)
				}
			} else {
				imageConfig.DensityCharacters = chars
			}

			fps, err := pconfig.GetInt("fps")
			if err != nil {
				if err != config.ErrNotFound {
					log.Warningf("failed to get 'fps' from config: %v", err)
				}
				fps = defaultFPS
			}
			device, err := capture.NewDevice(publisher, fps, imageConfig)
			if err != nil {
				return nil, fmt.Errorf("new device: %v", err)
			}
			return extutil.NopRedispatchHandler(browserapi.FuncHandler(
				handler.NopFromComponent(component.Sync(new(sync.Mutex), device)), device.Close)), nil
		},
		Command: textapi.CommandManual{
			Name: "rtcgetusermedia",
			Summary: "Opens up a new window with an ASCII-encoded feed of the user's default" +
				" video input device. This is a prototype that will be evolved into WebRTC " +
				"peer-to-peer calling system.",
		},
	})
	convertImageGrantee, _ := extutil.NewCommandSplitHandler(extutil.CommandSplitHandlerConfig{
		SplitOrientation: browserapi.OrientationRight,
		Handler: func(ctx context.Context, cmd textapi.Command,
			grants []extension.Grant, broker rpc.MuxBroker,
			invokeWindow browserapi.Window, config config.Config) (extutil.RedispatchHandler, error) {
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
			h := browserapi.NopHandler(handler.NopFromComponent(
				component.Sync(new(sync.Mutex), asciiart.NewComponent(img, cfg))))
			return extutil.NopRedispatchHandler(h), nil
		},
		Command: textapi.CommandManual{
			Name: "rtcconvertimage",
			Summary: "Converts a local JPEG image to an 130x70 ASCII encoded image and opens" +
				" up a window to display it.",
			Synopsis: "image",
		},
	})

	perms = append(perms, extensionapi.PermissionInterrupt)
	grantee := extutil.MultiGrantee(webcamGrantee, convertImageGrantee)
	return grantee, perms
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
