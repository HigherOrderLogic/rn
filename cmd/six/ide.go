package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ernestrc/blue/logging"
	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

// ide runs a terminal TUI session with a workspaceManagerHandler
type ide struct {
	ideConfig
	root      *workspaceManagerHandler
	clipboard *plugin.ClipboardManager
}

// newIde allocates storage for a new ide and initializes it with config
// at cfgfilename and filename. Note that if filename is empty, a default inmutable
// buffer will be loaded.
func newIde(cwd, cfgfilename string, filenames ...string) (
	i *ide, err error,
) {
	i = new(ide)
	err = i.init(cwd, cfgfilename, "", filenames...)
	return
}

// newIdeRecovery allocates storage for a new ide and initializes in recovery mode.
// The underlying editor will use recfilename to try to recover file at filename.
// Note that this function panics if either filename or recfilename are empty.
func newIdeRecovery(
	cwd, cfgfilename, filename string, recfilename string,
) (i *ide, err error) {
	if filename == "" || recfilename == "" {
		panic(fmt.Sprintf("invalid input: filename='%s', recfilename='%s'",
			filename, recfilename))
	}
	i = new(ide)
	err = i.init(cwd, cfgfilename, recfilename, filename)
	return
}

func (i *ide) init(cwd, cfgfilename, recfilename string, filenames ...string) error {
	configErr := loadConfig(&i.ideConfig, cfgfilename)

	cwdURI, err := workspace.ParseURI(cwd)
	if err != nil {
		return err
	}

	var l *log.Logger
	if i.ideConfig.logOutputPath() != "" {
		f, err := os.OpenFile(i.ideConfig.logOutputPath(),
			os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}

		level := i.ideConfig.logLevel()
		plugin.SetLoggingOutput(f)
		plugin.SetLoggingLevel(level)

		l = log.New()
		l.SetOutput(f)
		l.SetLevel(level)
		l.SetFormatter(&logging.LogrusFormatter{})
	} else {
		l = log.New()
		l.Out = ioutil.Discard
		l.Level = log.PanicLevel
	}

	i.clipboard = plugin.NewClipboardManager()

	root, err := newWorkspaceManagerHandler(l, i.clipboard, cwdURI,
		i.ideConfig, recfilename, filenames)
	if err != nil {
		return err
	}
	i.root = root
	logNonFatalErrs(l, configErr, i.ideConfig.errors)

	return nil
}

// run initialzes the underlying terminal environment and runs
// it with a workspace handler
func (i *ide) run() error {
	err := tui.Init()
	if err != nil {
		return err
	}

	term.SetOutputMode(i.ideConfig.outputMode())
	term.SetInputMode(i.ideConfig.inputMode())

	err = tui.RunWithLocker(i.root, &i.root.mu)
	if err != nil {
		return err
	}

	return nil
}

func (i *ide) closeResources() (ret error) {
	i.root.mu.Lock()
	defer i.root.mu.Unlock()

	if err := i.root.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	if err := i.clipboard.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	return
}

// Close satisfies io.Closer by closing this all ide's resources, including
// the terminal state.
func (i *ide) Close() error {
	tui.Close()
	return i.closeResources()
}
