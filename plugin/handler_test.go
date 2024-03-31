package plugin

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	browserapi "unstable.build/go-tui/api/browser"
	"unstable.build/go-tui/api/config"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	testutil "unstable.build/go-tui/util/test"
	"unstable.build/go-tui/workspace"
)

func TestPluginHandler(t *testing.T) {
	suite := []struct {
		description          string
		cmdAndArgs           string
		maxWidth             int
		frame                bool
		expectConstructorErr error
		drawnComponent       string
	}{
		{
			description: "no cmd and args runs a shell by default",
			cmdAndArgs:  "",
			maxWidth:    4,
			drawnComponent: `
 ◦     sh   0s
sh-3.2$       
              
              
              
              `,
		},
		{
			description: "command with arg",
			cmdAndArgs:  "sleep 2",
			maxWidth:    4,
			drawnComponent: `
 ◦  sleep 2 0s
              
              
              
              
              `,
		},
		{
			description: "max width 0 doesn't panic",
			cmdAndArgs:  "sleep 2",
			maxWidth:    0,
			drawnComponent: `
 ◦  sleep 2 0s
              
              
              
              
              `,
		},
	}

	// important so test correctness doesn't depend on host
	os.Setenv("SHELL", "sh")

	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			ctx := context.Background()
			tempDir, err := ioutil.TempDir("", "")
			require.NoError(t, err)
			uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
			require.NoError(t, err)
			fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
			require.NoError(t, err)

			ch := make(chan struct{})
			waitInterrupt := term.FuncInterrupter(func(context.Context) error {
				select {
				case ch <- struct{}{}:
				default:
				}
				return nil
			})
			h, err := New(nopBrowser{interrupt: waitInterrupt}, nopBrowser{}, fileScheme,
				fileScheme, nopBrowser{}, vte.DefaultConfig(), test.cmdAndArgs, test.maxWidth,
				test.frame, component.FrameCharSetDefault(), term.Attributes{})
			require.Equal(t, test.expectConstructorErr, err)
			h.Resize(14, 6)

			w := term.NewStringWriter(14, 6)

			tests := []testutil.ComponentTestCase{
				{Action: nil, Expected: test.drawnComponent},
			}

			<-ch
			testutil.TestComponent(t, h, w, tests)
		})
	}
}

type nopBrowser struct {
	interrupt term.Interrupter
}

func (n nopBrowser) PublishEvent(ev term.Event) error {
	if ev.Type != term.EventInterrupt {
		return errors.New("unexpected event type")
	}
	if n.interrupt != nil {
		n.interrupt.Interrupt(context.Background())
	}
	return nil
}

func (n nopBrowser) Notify(notifications.Level, string, ...interface{}) error {
	return nil
}

func (n nopBrowser) Tab(uri workspaceapi.URI, name string, h browserapi.Handler) (
	browserapi.Handler, error,
) {
	panic("should not be called")
}

func (n nopBrowser) SetTabName(workspaceapi.URI, string, term.Attributes) error {
	panic("should not be called")
}
