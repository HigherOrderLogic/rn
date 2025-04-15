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

package ide

import (
	"fmt"

	textapi "unstable.build/go-tui/api/text"
)

const (
	cmdEdit                   = "edit"
	cmdReadFile               = "readFile"
	cmdChangeSplitOrientation = "changeSplitOrientation"
	cmdSplitWindow            = "splitWindow"
	cmdNewWindow              = "newWindow"
	cmdSetDefaultColors       = "setDefaultColors"
	cmdSwitchToTab            = "switchToTab"
	cmdFocusWindow            = "focusWindow"
	cmdMoveWindow             = "moveWindowContent"
	cmdResizeWindow           = "resizeWindow"
	cmdCopyPath               = "copyPath"
)

type commandAll struct {
	man     textapi.CommandManual
	handler func(*ex, ...string) error
}

var (
	exCommands = map[string]commandAll{
		"renameTab": {
			man: textapi.CommandManual{
				Summary:  "Rename the current tab in focus. Optionally set the colors of the tab title.",
				Synopsis: "name [foreground [background]]",
			},
			handler: (*ex).renameTab,
		},
		"echo": {
			man: textapi.CommandManual{
				Summary: "Replay the given sequence of keys back into the event loop " +
					"as if the user had dispatched them. This allows for building macros that " +
					"perform tasks that couldn't be accomplished with " +
					"combinations of commands alone. For instance `echo :edit` opens " +
					"the command prompt with a pre-populated command. The syntax of " +
					"non-character keys is the same used in the `key_bindings` section " +
					"of the config. There's a special {wait} instruction that can be " +
					"interleaved to deterministically wait for the command prompt " +
					"auto-completer to finish populating the search list before " +
					"processing the next key.",
				Synopsis: "sequence",
			},
			handler: (*ex).echo,
		},
		"previousTab": {
			man: textapi.CommandManual{
				Summary: "Set the content of the current active window to the previous tab in the tabs list. " +
					"Wraps around the start of the tabs list.",
			},
			handler: (*ex).previousTab,
		},
		"nextTab": {
			man: textapi.CommandManual{
				Summary: "Set the content of the current active window to the next tab in the tabs list. " +
					"Wraps around the end of the tabs list.",
			},
			handler: (*ex).nextTab,
		},
		"closeTab": {
			man: textapi.CommandManual{
				Summary: "Close the current active window's tab. It automatically replaces it " +
					"with the next available tab in the tabs list.",
			},
			handler: (*ex).closeTab,
		},
		cmdSwitchToTab: {
			man: textapi.CommandManual{
				Summary: "Set the content of the current active window to the tab at the given " +
					"position in the tabs list.",
				Synopsis: "number",
			},
			handler: (*ex).switchToTab,
		},
		"closeAllTabs": {
			man: textapi.CommandManual{
				Summary: "Closes all tabs in the tabs list.",
			},
			handler: (*ex).closeAllTabs,
		},
		"closeInactiveTabs": {
			man: textapi.CommandManual{
				Summary: "Closes all tabs that aren't used by any window.",
			},
			handler: (*ex).closeInactiveTabs,
		},
		"closeWindow": {
			man: textapi.CommandManual{
				Summary: "Closes the current active window and switches focus " +
					"to the next available window. This command fails if there's only one " +
					"window remaining.",
			},
			handler: (*ex).closeFocusWindow,
		},
		"closeOtherWindows": {
			man: textapi.CommandManual{
				Summary: "Closes all the windows except the current active window. " +
					"This command fails if there's only one window remaining.",
			},
			handler: (*ex).closeOtherWindows,
		},
		"writeQuit": {
			man: textapi.CommandManual{
				Summary: "Write the current file to disk and exits if and only if " +
					"there are no files with changes pending to be written to disk.",
			},
			handler: (*ex).flushClose,
		},
		"writeForceQuit!": {
			man: textapi.CommandManual{
				Summary: "Write the current file to disk and exits. If there are files with " +
					"pending changes, these are ignored and stashed away.",
			},
			handler: (*ex).flushCloseIgnoreNonFlushed,
		},
		"write!": {
			man: textapi.CommandManual{
				Summary: "Write the current file to disk with any pending changes along with it. " +
					"This is the standard way to save changes to a file. It fails if file was " +
					"open read-only or when there is another reason why the file can't be written.",
			},
			handler: (*ex).forceFlush,
		},
		"forceQuit!": {
			man: textapi.CommandManual{
				Summary: "Exit without writing any pending changes to disk.",
			},
			handler: (*ex).forceQuit,
		},
		"quit": {
			man: textapi.CommandManual{
				Summary: "Exit if and only if there are no files with changes pending to be " +
					"written to disk.",
			},
			handler: (*ex).quit,
		},
		"reloadFile!": {
			man: textapi.CommandManual{
				Summary: "Reloads the file in the current active window, if it is a workspace file.",
			},
			handler: (*ex).reloadFile,
		},
		cmdChangeSplitOrientation: {
			man: textapi.CommandManual{
				Summary: "Toggle the next window split orientation or change it to the " +
					"given orientation if passed via arguments. The options are 'horizontal' which " +
					"sets the next split to be below the current active window or 'vertical' which " +
					"sets the next split to be right of the current active window.",
				Synopsis: "(horizontal|vertical)",
			},
			handler: (*ex).splitDirectionChange,
		},
		cmdSplitWindow: manSplitWindow,
		cmdNewWindow:   manSplitWindow,
		cmdFocusWindow: {
			man: textapi.CommandManual{
				Summary:  "Switches the window focus to the window on the given side of the current active window.",
				Synopsis: "(right|left|up|down)",
			},
			handler: (*ex).focusWindow,
		},
		cmdMoveWindow: {
			man: textapi.CommandManual{
				Summary:  "Moves the content of the window in focus to the window in the given direction.",
				Synopsis: "(right|left|up|down)",
			},
			handler: (*ex).moveWindow,
		},
		cmdResizeWindow: {
			man: textapi.CommandManual{
				Summary:  "Resizes the window by increasing or decreasing its width or height.",
				Synopsis: "(increase|decrease|max|min|reset) (height|width)",
			},
			handler: (*ex).resizeWindow,
		},
		"toggleFullscreen": {
			man: textapi.CommandManual{
				Summary: "This is a toggle version of 'resizeWindow max height+width'." +
					"A subsequent invocation of this command will effectively reset the " +
					"window size via 'resizeWindow reset'. Shifting the focus to another " +
					"window also resets the size of the current window in fullscreen.",
			},
			handler: (*ex).toggleFullscreen,
		},
		"notificationsSendInfo": {
			man: textapi.CommandManual{
				Summary:  "Sends an info-level notification.",
				Synopsis: "message",
			},
			handler: (*ex).sendNotificationInfo,
		},
		"notificationsSendSuccess": {
			man: textapi.CommandManual{
				Summary:  "Sends a success-level notification.",
				Synopsis: "message",
			},
			handler: (*ex).sendNotificationSuccess,
		},
		"notificationsSendWarning": {
			man: textapi.CommandManual{
				Summary:  "Sends a warning-level notification.",
				Synopsis: "message",
			},
			handler: (*ex).sendNotificationWarning,
		},
		"notificationsSendError": {
			man: textapi.CommandManual{
				Summary:  "Sends an error-level notification.",
				Synopsis: "message",
			},
			handler: (*ex).sendNotificationError,
		},
		"notificationsCloseAll": {
			man: textapi.CommandManual{
				Summary: "Closes all active notifications rendered by the browser.",
			},
			handler: (*ex).closeNotifications,
		},
		"notificationsPauseAll": {
			man: textapi.CommandManual{
				Summary: "Pauses automatic closure of all active notifications rendered by the browser.",
			},
			handler: (*ex).pauseNotifications,
		},
		"notificationsResumeAll": {
			man: textapi.CommandManual{
				Summary: "Resumes automatic closure of all previously paused notifications rendered by the browser.",
			},
			handler: (*ex).resumeNotifications,
		},
		"panic": {
			man: textapi.CommandManual{
				Summary: "Causes the editor to panic. This is internal and for debugging purposes only.",
			},
			handler: (*ex).panic,
		},
		"newTerminalTab": {
			man: textapi.CommandManual{
				Summary: "Opens a new terminal emulator in a new tab and attaches it to the current " +
					"active window. If 'shell' is not set in " +
					"terminal config, then the default system shell defined via SHELL " +
					"environment variable is used.",
			},
			handler: (*ex).newTerminalTab,
		},
		"newTerminal": {
			man: textapi.CommandManual{
				Summary: "Opens a new terminal emulator and attaches it to the current " +
					"active window. The terminal created by this command is automatically " +
					"closed when the content of the window is updated for example by " +
					"'nextTab' or 'previousTab'. If 'shell' is not set in " +
					"terminal config, then the default system shell defined via SHELL " +
					"environment variable is used.",
			},
			handler: (*ex).newTerminalTab,
		},
		"newTerminalOrSplit": {
			man: textapi.CommandManual{
				Summary: "Opens a new terminal emulator and attaches it to the " +
					"current active window if empty, or creates a new split window if " +
					"the window is not empty. The terminal created by this " +
					"command is automatically closed when the content of the " +
					"window is updated for example by " +
					"'nextTab' or 'previousTab'. If 'shell' is not set in " +
					"terminal config, then the default system shell defined via SHELL " +
					"environment variable is used.",
			},
			handler: (*ex).newTerminalOrSplit,
		},
		cmdEdit: {
			man: textapi.CommandManual{
				Summary: "Opens the file at the given URI for editing on the current active " +
					"window, replacing its contents. If no scheme is provided, file:// " +
					"is used by default. This allows a user opening files in " +
					"workspaces outside the current workspace or host. " +
					"A .swp file is created in the same folder to prevent multiple sessions " +
					"from overriding each others changes. " +
					"If file has any pending changes that were lost due to a crash or " +
					"there's another session currently editing the file, a prompt is opened " +
					"to come to a decision.",
				Synopsis: "[scheme:][//[userinfo@]host][/]filepath",
			},
			handler: (*ex).editFiles,
		},
		cmdCopyPath: {
			man: textapi.CommandManual{
				Summary:  "Copies the current editor's URI path.",
				Synopsis: "[absolute]",
			},
			handler: (*ex).copyPath,
		},
		"!": {
			man: textapi.CommandManual{
				Summary: "Opens a new terminal emulator with the given executable " +
					"and arguments in a new floating window. " +
					"The stdout and stderr of the execution " +
					"are printed on the window along with stats and a progress sign until " +
					"user closes the window or hits the ESC key. \n\n" +
					"If no executable is passed, this command opens the companion terminal emulator" +
					"The companion terminal emulator is different " +
					"than a terminal emulator created by newTerminalTab in that it preserves " +
					"the session output accross invocations. The floating window created as a " +
					"result of this command can be closed via standard window or tab close commands.",
				Synopsis: "[executable [args]]",
			},
			handler: (*ex).executePlugin,
		},
		"clipboardCopy": {
			man: textapi.CommandManual{
				Summary: "Copies the selected text into the configured clipboard. ",
			},
			handler: (*ex).copyToClipboard,
		},
		"clipboardPaste": {
			man: textapi.CommandManual{
				Summary: "Paste the last text copied into the configured clipboard. ",
			},
			handler: (*ex).pasteFromClipboard,
		},
		cmdSetDefaultColors: {
			man: textapi.CommandManual{
				Summary: "Changes the default background and optionally foreground colors of " +
					"the window in focus. The color can be a named color or an RGB value " +
					"in hexadecimal notation (i.e. #FFFFFF).",
				Synopsis: "background [foreground]",
			},
			handler: (*ex).setDefaultColors,
		},
		cmdReadFile: {
			man: textapi.CommandManual{
				Summary: "Insert the contents of the passed file name below the cursor. Takes in " +
					"a uri with a scheme as an argument. If no scheme is passed `file://` is assumed",
				Synopsis: "[scheme:][//[userinfo@]host][/]filepath",
			},
			handler: (*ex).readFile,
		},
	}

	manSplitWindow = commandAll{
		man: textapi.CommandManual{
			Summary: "Splits the current active window vertically or horizontally in two, " +
				"changing the window focus to it. " +
				"If no orientation is passed, the default split orientation is used. " +
				fmt.Sprintf("Check %s for more details on how changing the "+
					"default orientation works.", cmdChangeSplitOrientation),
			Synopsis: "[right|left|up|down]",
		},
		handler: (*ex).newWindow,
	}

	runShaderCmdManual = textapi.CommandManual{
		Name: "runShader",
		Summary: "Run a shader from the library of shaders. " +
			"By default duration is 1s and fps is 30.",
		Synopsis: "name [duration] [fps]",
	}
)
