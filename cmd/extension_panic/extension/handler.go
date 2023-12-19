package extension

import (
	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	"unstable.build/go-tui/api/config"
	textapi "unstable.build/go-tui/api/text"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/extension"
	extutil "unstable.build/go-tui/extension/util"
	"unstable.build/go-tui/proto"
	"unstable.build/go-tui/term"
)

// Grantee returns this extension's Grantee and the permissions required to run it.
func Grantee() (extension.Grantee, []extension.Permission) {
	return extutil.NewCommandSplitHandler(extutil.CommandSplitHandlerConfig{
		SplitOrientation: browserapi.OrientationRight,
		Handler: func(cmd textapi.Command, grants []extension.Grant, broker proto.MuxBroker,
			invokeWindow browserapi.Window, config config.Config) (browserapi.Handler, error) {
			return new(panicHandler), nil
		},
		Command: textapi.CommandManual{
			Name:    "panicExtension",
			Summary: "Causes the extension to panic. This is internal and for debugging purposes only.",
		},
	})
}

type panicHandler struct {
	comp tui.Component
}

func (h *panicHandler) Resize(width, height int) {
	if h.comp == nil {
		h.comp = component.NewStringWithConfig(`
              . . .                         
              \|/                          
            '--+--'                        
              /|\                          
             ' | '                         
               |                           
               |                           
           ,--'#'--.                       
           |#######|                       
        _.-'#######'-._                    
     ,-'###############'-.                 
   ,'#####################',               
  /#########################\              
 |###########################|             
|#############################|            
|#############################|            
|#############################|            
|#############################|            
 |###########################|             
  \#########################/              
   '.#####################,'               
     '._###############_,'                 
        '--..#####..--'
`, component.StringConfig{
			Alignment: component.SpanAlignmentCentered,
		})
	}
	h.comp.Resize(width, height)
}

func (h *panicHandler) Draw(w term.Writer) {
	h.comp.Draw(w)
}

func (h *panicHandler) Handle(ev term.Event) (exit, handled bool) {
	panic("kaboom")
}

func (h *panicHandler) Cursor() (
	pos term.Coordinates, style term.CursorStyle, show bool,
) {
	return
}

func (h *panicHandler) Man() tui.Manual {
	return tui.Manual{}
}

func (h *panicHandler) Close() error {
	return nil
}
