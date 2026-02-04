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

package text

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// SubscribeLocationCommands returns a slice of location list related commands
// that can be registered for the returned CommandHandler.
func SubscribeLocationCommands(
	file workspaceapi.URI, registry FileCommandRegistry, ed Handler,
) (Handler, error) {
	ret := locationCommandHandler{
		file:              file,
		registry:          registry,
		Handler:           ed,
		userLocationLists: make(map[string]struct{}),
	}
	var retErr error
	for _, cmd := range locationCommands {
		if err := registry.SubscribeCommandForFile(file, cmd, ret); err != nil {
			retErr = multierror.Append(retErr, err)
		}
	}
	if retErr != nil {
		return nil, retErr
	}
	return ret, nil
}

const (
	// CommandLocationJump is the command name for jumping to locations.
	CommandLocationJump = "jumptolocation"
	// CommandCreateLocation is the command name for creating locations.
	CommandCreateLocation = "locationcreate"
	// CommandDeleteAllLocations is the command name for deleting all locations on a list.
	CommandDeleteAllLocations = "locationdeleteall"
	commandToggleLocation     = "locationtoggle"
	commandDeleteLocation     = "locationdelete"
	defaultUserLocationList   = "mark"
)

var locationCommands = []textapi.CommandManual{
	{
		Name:     CommandLocationJump,
		Summary:  "Jumps to locations on the given location list.",
		Synopsis: "(next|prev) <location-list>",
		Commands: []textapi.CommandManual{
			{
				Name:     "next",
				Summary:  "Jumps to the next location.",
				Synopsis: "<location-list>",
			},
			{
				Name:     "previous",
				Summary:  "Jumps to the previous location.",
				Synopsis: "<location-list>",
			},
		},
	},
	{
		Name: CommandCreateLocation,
		Summary: fmt.Sprintf("Saves the current cursor location as a location that can be "+
			"used to jump to via `locationjump %[1]s`. By default, the location list name is `%[1]s` "+
			"but this can be overriden by passing a location list name.",
			defaultUserLocationList),
		Synopsis: "[location-list]",
	},
	{
		Name: commandToggleLocation,
		Summary: fmt.Sprintf("Creates or deletes the current cursor location as a location that can be "+
			"used to jump to via `locationjump %[1]s`. By default, the location list name is `%[1]s` "+
			"but this can be overriden by passing a location list name.",
			defaultUserLocationList),
		Synopsis: "[location-list]",
	},
	{
		Name: commandDeleteLocation,
		Summary: fmt.Sprintf("Delete the current cursor location in the given location list. "+
			"By default, the location list name is `%[1]s` "+
			"but this can be overriden by passing a location list name.", defaultUserLocationList),
		Synopsis: "[location-list]",
	},
	{
		Name: CommandDeleteAllLocations,
		Summary: fmt.Sprintf("Removes all of the locations of the given location list. The default location"+
			" list is `%s`.", defaultUserLocationList),
		Synopsis: "[location-list]",
	},
}

type locationCommandHandler struct {
	file     workspaceapi.URI
	registry FileCommandRegistry
	Handler
	userLocationLists map[string]struct{}
}

func (u locationCommandHandler) Close() (ret error) {
	ret = u.Handler.Close()
	for _, cmd := range locationCommands {
		err := u.registry.UnsubscribeCommandForFile(u.file, cmd.Name)
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

func (u locationCommandHandler) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) (err error) {
	switch cmd.Name {
	case CommandLocationJump:
		err = u.handleLocationJump(cmd)
	case CommandCreateLocation:
		err = u.handleCreateLocation(cmd)
	case commandDeleteLocation:
		err = u.handleDeleteLocation(cmd)
	case commandToggleLocation:
		err = u.handleDeleteLocation(cmd)
		if err != nil {
			err = u.handleCreateLocation(cmd)
		}
	case CommandDeleteAllLocations:
		err = u.handleDeleteAllLocations(cmd)
	default:
		err = errors.New("extraneous command")
	}
	return
}

func (u locationCommandHandler) Complete(ctx context.Context, cmd textapi.Command) (
	ret iterator.Iterator[string], _ string, err error,
) {
	switch cmd.Name {
	case CommandLocationJump:
		ret, err = u.completeLocationJump(cmd)
	case CommandCreateLocation, commandToggleLocation:
		ret, err = u.completeCreateLocation(cmd)
	case commandDeleteLocation:
		ret, err = u.completeDeleteLocation(cmd)
	case CommandDeleteAllLocations:
		ret, err = u.completeCreateLocation(cmd)
	default:
		err = errors.New("extraneous command")
	}
	return
}

func (u locationCommandHandler) handleLocationJump(cmd textapi.Command) error {
	if len(cmd.Args) == 0 {
		return errors.New("missing next/previous and location list id")
	}
	if len(cmd.Args) == 1 {
		return errors.New("missing location list id")
	}
	lists := u.Handler.LocationLists()
	for _, list := range lists {
		if list.ID != cmd.Args[1] {
			continue
		}
		switch cmd.Args[0] {
		case "next":
			ok := u.Handler.MoveToNextLocation(cmd.Args[1])
			if !ok {
				log.Debugf("reached end of location list")
			}
			return nil
		case "previous":
			ok := u.Handler.MoveToPrevLocation(cmd.Args[1])
			if !ok {
				log.Debugf("reached start of location list")
			}
			return nil
		}
		return errors.New("only 'next' or 'previous' is accepted")
	}
	return errors.New("location list does not exist")
}

func (u locationCommandHandler) completeLocationJump(
	cmd textapi.Command,
) (ret iterator.Iterator[string], err error) {
	if len(cmd.Args) == 1 {
		ret = iterator.FromSlice([]string{"previous", "next"})
		return
	}
	lists := u.Handler.LocationLists()
	if len(cmd.Args) == 2 {
		var ids []string
		for _, list := range lists {
			// lists starting with _ are not displayed.
			// This is useful to hide lists that might
			// be needed for internal implementations.
			if strings.HasPrefix(list.ID, "_") {
				continue
			}
			ids = append(ids, list.ID)
		}
		ret = iterator.FromSlice(ids)
		return
	}
	ret = iterator.Empty[string]()
	return
}

func (u locationCommandHandler) getCursorMark() (term.Coordinates, term.Coordinates) {
	cursor := u.CursorAtScroll()
	return cursor, term.Coordinates{Y: cursor.Y, X: cursor.X + 1}
}

func (u locationCommandHandler) handleDeleteAllLocations(cmd textapi.Command) (err error) {
	list := defaultUserLocationList
	if len(cmd.Args) > 0 {
		list = cmd.Args[0]
	}
	u.Handler.SetLocationList(textapi.LocationPriorityInfo, list, nil)
	clear(u.userLocationLists)
	return
}

func (u locationCommandHandler) handleDeleteLocation(cmd textapi.Command) (err error) {
	list := defaultUserLocationList
	if len(cmd.Args) > 0 {
		list = cmd.Args[0]
	}
	var curr []textapi.Location
	lists := u.Handler.LocationLists()
	for _, l := range lists {
		if l.ID == list {
			curr = l.Locations
		}
	}
	from, to := u.getCursorMark()
	var success bool
	for i, loc := range curr {
		if loc.From == from && loc.To == to {
			curr[i] = curr[len(curr)-1]
			curr = curr[:len(curr)-1]
			success = true
			break
		}
	}
	if !success {
		return fmt.Errorf("there's no location at the given cursor position for given location list")
	}
	sort.Slice(curr, func(i, j int) bool {
		res := term.CoordinatesDiff(curr[i].From, curr[j].From)
		return res.Y < 0 || res.Y == 0 && res.X < 0
	})
	u.Handler.SetLocationList(textapi.LocationPriorityInfo, list, LocationSlice(curr))
	if list != defaultUserLocationList && len(curr) == 0 {
		delete(u.userLocationLists, list)
	}
	return
}

func (u locationCommandHandler) completeDeleteLocation(
	cmd textapi.Command,
) (ret iterator.Iterator[string], err error) {
	if len(cmd.Args) == 1 {
		lists := u.Handler.LocationLists()
		from, to := u.getCursorMark()
		for _, list := range lists {
			_, isUser := u.userLocationLists[list.ID]
			if !isUser && list.ID != defaultUserLocationList {
				continue
			}
			for _, loc := range list.Locations {
				if loc.From == from && loc.To == to {
					return iterator.FromSlice([]string{list.ID}), nil
				}
			}
		}
	}
	// avoid history completion
	ret = iterator.FromSlice([]string{""})
	return
}

func (u locationCommandHandler) handleCreateLocation(cmd textapi.Command) (err error) {
	list := defaultUserLocationList
	if len(cmd.Args) > 0 {
		list = cmd.Args[0]
	}
	var curr []textapi.Location
	lists := u.Handler.LocationLists()
	for _, l := range lists {
		if l.ID == list {
			curr = l.Locations
		}
	}
	from, to := u.getCursorMark()
	curr = append(curr, textapi.Location{
		From: from,
		To:   to,
		Attr: term.Attributes{
			Bg: tcell.ColorGray,
		},
	})
	u.Handler.SetLocationList(textapi.LocationPriorityInfo, list, LocationSlice(curr))
	if list != defaultUserLocationList {
		u.userLocationLists[list] = struct{}{}
	}
	return
}

func (u locationCommandHandler) completeCreateLocation(
	cmd textapi.Command,
) (ret iterator.Iterator[string], err error) {
	if len(cmd.Args) == 1 {
		locationLists := []string{defaultUserLocationList}
		for id := range u.userLocationLists {
			locationLists = append(locationLists, id)
		}
		ret = iterator.FromSlice(locationLists)
		return
	}
	ret = iterator.Empty[string]()
	return
}
