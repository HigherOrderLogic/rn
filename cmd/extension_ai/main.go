package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"

	"github.com/ernestrc/blue/logging"
	"github.com/sashabaranov/go-openai/jsonschema"
	log "github.com/sirupsen/logrus"
	configapi "unstable.build/go-tui/api/config"
	"unstable.build/go-tui/cmd/extension_ai/backend"
	"unstable.build/go-tui/cmd/extension_ai/backend/openai"
	"unstable.build/go-tui/cmd/extension_ai/dialogue"
	"unstable.build/go-tui/cmd/extension_ai/extension"
	"unstable.build/go-tui/extension/process"
)

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:8886", nil))
	}()
	openaiSvc := func(config configapi.Config, model string) (backend.Service, error) {
		apiKey, err := config.GetString("api_key")
		if err != nil {
			err = fmt.Errorf("failed to get 'api_key' from config: %w", err)
			return nil, err
		}
		return openai.NewClient(apiKey, openai.Config{
			Model: model,
			Tools: editorTools(),
		}, availableModels()), nil
	}
	grantee, perms := extension.GranteeWithService(openaiSvc,
		availableModels(), defaultModel,
		dialogue.WithCompleter(dialogue.FuncCompleter(logCompletion)),
		dialogue.WithInitialContext([]backend.ChatCompletionMessage{
			{
				Role: backend.RoleSystem,
				Content: "You are a helpful coding assistant. Any code changes must be " +
					" answered in diff format.",
			},
		}),
	)
	process.Serve(grantee, perms...)
}

var defaultModel = openai.GPT4Turbo

func logCompletion(
	ctx context.Context, dialogueID, completionID string,
	finishReason backend.FinishReason,
	msg backend.ChatCompletionMessage,
) {
	log.WithFields(log.Fields{
		"reason":            finishReason,
		"dialogueID":        dialogueID,
		"completionID":      completionID,
		logging.KeyCallType: "logCompletion",
		logging.KeyFile:     "main.go",
	}).Debugf("%+v", msg)
}

func availableModels() map[string]int {
	return openai.AvailableModels()
}

func editorTools() []openai.Tool {
	return []openai.Tool{}
	/*return []openai.Tool{
		{Type: openai.ToolTypeFunction, Function: openai.FunctionDefinition{
			Name: "editFile",
			Description: fmt.Sprintf("Edit replaces any file content between a 'start' and 'end' " +
				"coordinates. If 'start' and 'end' are equal, then content is just inserted. " +
				"For instance, if a file's content was \"hello\nworld\" " +
				"and we performed 'editFile' with 'start' coordinates of line 0 column 4 and " +
				"'end' coordinates of  line 1, column 1, with 'content' \"u v\", " +
				"the resulting content in the file would be \"hellu vorld\"."),
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"start": {
						Type:        jsonschema.Object,
						Description: "left-inclusive start coordinates",
						Properties:  coordinatesProperties(),
						Required:    coordinatesRequired(),
					},
					"end": {
						Type:        jsonschema.Object,
						Description: "right-exclusive end coordinates",
						Properties:  coordinatesProperties(),
						Required:    coordinatesRequired(),
					},
					"content": {
						Type:        jsonschema.String,
						Description: "replacement content",
					},
				},
				Required: []string{"start", "end", "content"},
			},
		}},
	}*/
}

func coordinatesProperties() map[string]jsonschema.Definition {
	return map[string]jsonschema.Definition{
		"column": {
			Type: jsonschema.Integer,
			Description: "zero-based character index for a given line. " +
				"For instance in the string 'hello\nworld', the character 'o' is at column 4",
		},
		"line": {
			Type: jsonschema.Object,
			Description: "zero-based line index. " +
				"For instance, in the string " +
				"'hello\nworld', the character 'd' is at line 1.",
		},
	}
}

func coordinatesRequired() []string {
	return []string{"line", "column"}
}
