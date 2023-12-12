package extension

import (
	"context"
	"errors"
	"fmt"

	"github.com/ernestrc/blue/logging"
	log "github.com/sirupsen/logrus"
	configapi "unstable.build/go-tui/api/config"
	textapi "unstable.build/go-tui/api/text"
	"unstable.build/go-tui/cmd/extension_ai/backend"
	"unstable.build/go-tui/cmd/extension_ai/backend/openai"
	"unstable.build/go-tui/cmd/extension_ai/dialogue"
	"unstable.build/go-tui/extension"
	extutil "unstable.build/go-tui/extension/util"
	"unstable.build/go-tui/proto"
)

// GranteeWithService returns this extension's grantee, with the given
// backend.Service constructor as the backend servicing the LLM.
func GranteeWithService(
	svcFunc func(configapi.Config, map[string]int, string) (backend.Service, error),
	defaultAvailableModels map[string]int,
	defaultModel string,
	options ...dialogue.Option,
) (extension.Grantee, []extension.Permission) {
	commandEventHandler := func(
		ed textapi.Editor, grants []extension.Grant,
		broker proto.MuxBroker, pconfig configapi.Config,
	) (hret extutil.CommandEventHandler, err error) {
		return CommandEventHandler(ed, grants, broker, pconfig,
			svcFunc, defaultAvailableModels, defaultModel, options...)
	}
	grantee, perms := extutil.NewEditorEventHandler(
		AIHandlerCommands, commandEventHandler, AIHandlerEvents,
		AIHandlerPermissions...)
	return grantee, perms
}

const (
	defaultDefaultModel = openai.GPT3Dot5Turbo
	defaultBaseURL      = "" // uses openai's default base URL
)

// DefaultOpenAIGrantee returns GranteeWithService satisfied by an OpenAI-like
// backend service. Configuration can set a 'base_url' to override openai's
// service URL with a custom one.
func DefaultOpenAIGrantee() (extension.Grantee, []extension.Permission) {
	openaiSvc := func(
		config configapi.Config, availableModels map[string]int, model string,
	) (backend.Service, error) {
		apiKey, err := config.GetString("api_key")
		if err != nil {
			err = fmt.Errorf("failed to get 'api_key' from config: %w", err)
			return nil, err
		}
		openaiConfig := openai.Config{
			Model: model,
		}
		openaiConfig.BaseURL, err = config.GetString("base_url")
		if err != nil {
			if !errors.Is(err, configapi.ErrNotFound) {
				err = fmt.Errorf("failed to get 'base_url' from config: %w", err)
				return nil, err
			}
			openaiConfig.BaseURL = defaultBaseURL
		}
		openaiConfig.FrequencyPenalty, err = config.GetFloat("frequency_penalty")
		if err != nil {
			if !errors.Is(err, configapi.ErrNotFound) {
				err = fmt.Errorf("failed to get 'frequency_penalty' from config: %w", err)
				return nil, err
			}
		}
		openaiConfig.MaxTokens, err = config.GetInt("max_tokens")
		if err != nil {
			if !errors.Is(err, configapi.ErrNotFound) {
				err = fmt.Errorf("failed to get 'max_tokens' from config: %w", err)
				return nil, err
			}
		}
		openaiConfig.PresencePenalty, err = config.GetFloat("presence_penalty")
		if err != nil {
			if !errors.Is(err, configapi.ErrNotFound) {
				err = fmt.Errorf("failed to get 'presence_penalty' from config: %w", err)
				return nil, err
			}
		}
		openaiConfig.Temperature, err = config.GetFloat("temperature")
		if err != nil {
			if !errors.Is(err, configapi.ErrNotFound) {
				err = fmt.Errorf("failed to get 'temperature' from config: %w", err)
				return nil, err
			}
		}
		openaiConfig.TopP, err = config.GetFloat("top_p")
		if err != nil {
			if !errors.Is(err, configapi.ErrNotFound) {
				err = fmt.Errorf("failed to get 'top_p' from config: %w", err)
				return nil, err
			}
		}
		return openai.NewClient(apiKey, openaiConfig, availableModels), nil
	}
	return GranteeWithService(openaiSvc,
		openai.AvailableModels(), defaultDefaultModel,
		dialogue.WithCompleter(dialogue.FuncCompleter(logCompletion)),
		dialogue.WithInitialContext([]backend.ChatCompletionMessage{
			{
				Role: backend.RoleSystem,
				Content: "You are a helpful coding assistant. Any code changes must be " +
					" answered in diff format.",
			},
		}),
	)
}

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
