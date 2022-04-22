package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ernestrc/blue/logging"
	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text/vi"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

// IDE binds together a text editor/browser with a plugin manager.
type IDE struct {
	ideConfig
	root      *workspaceHandler
	clipboard *plugin.ClipboardManager
}

// New allocates storage for a new IDE and initializes it with config
// at cfgfilename and filename. Note that if filename is empty, a default inmutable
// buffer will be loaded.
func New(cwd, cfgfilename string, filenames ...string) (
	i *IDE, err error,
) {
	i = new(IDE)
	err = i.init(true, cwd, cfgfilename, "", filenames...)
	return
}

// NewRecovery allocates storage for a new IDe and initializes in recovery mode.
// The underlying editor will use recfilename to try to recover file at filename.
// Note that this function panics if either filename or recfilename are empty.
func NewRecovery(
	cwd, cfgfilename, filename string, recfilename string,
) (i *IDE, err error) {
	if filename == "" || recfilename == "" {
		panic(fmt.Sprintf("invalid input: filename='%s', recfilename='%s'",
			filename, recfilename))
	}
	i = new(IDE)
	err = i.init(true, cwd, cfgfilename, recfilename, filename)
	return
}

func (i *IDE) init(initTUI bool, cwd, cfgfilename, recfilename string, filenames ...string) error {
	configErr := loadConfig(&i.ideConfig, cfgfilename)

	cwdURI, err := workspace.ParseURI(cwd)
	if err != nil {
		return err
	}

	var viOpts []vi.Option

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
		viOpts = append(viOpts, vi.WithLogger(l))
	} else {
		l = log.New()
		l.Out = ioutil.Discard
		l.Level = log.PanicLevel
	}

	i.clipboard = plugin.NewClipboardManager()

	viOpts = append(viOpts,
		vi.WithResAttr(i.ideConfig.viResultAttr()),
		vi.WithDebug(i.ideConfig.viDebug()),
		vi.WithWrap(i.ideConfig.viWrap()),
		vi.WithClipboard(i.clipboard),
	)

	// TODO
	var msg browser.Messenger

	if initTUI {

		vi := vi.Editor(viOpts...)

		root, err := newHandler(vi, msg, l, i.clipboard, cwdURI,
			i.ideConfig, recfilename, filenames)
		if err != nil {
			return err
		}
		i.root = root
		err = tui.Init()
		if err != nil {
			return err
		}

		term.SetOutputMode(i.ideConfig.outputMode())
		term.SetInputMode(i.ideConfig.inputMode())
	}

	reportNonFatalErrs(l, msg, configErr, i.ideConfig.errors)
	return nil
}

func reportNonFatalErrs(
	l *log.Logger, messenger browser.Messenger,
	configErr error,
	configErrs map[string]error,
) {
	all := configErr
	for key, err := range configErrs {
		err = fmt.Errorf("Failed to load %q: %v", key, err)
		all = multierr.Append(all, err)
	}
	if l != nil && all != nil {
		l.Warn(all)
	}
	if messenger != nil {
		messenger.SetMessage("Error: %s", all)
	}
}

// Run initialzes the underlying terminal environment and runs
// it with this tui.Handler.
func (i *IDE) Run() error {
	err := tui.RunWithLocker(i.root, &i.root.mu)
	if err != nil {
		return err
	}

	return nil
}

func (i *IDE) closeResources() (ret error) {
	if i.root != nil {
		if err := i.root.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
	}

	if err := i.clipboard.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	return
}

// Close satisfies io.Closer by closing this all IDE's resources, including
// the environment terminal state.
func (i *IDE) Close() error {
	tui.Close()
	return i.closeResources()
}
