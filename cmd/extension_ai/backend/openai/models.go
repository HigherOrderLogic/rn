package openai

import "github.com/sashabaranov/go-openai"

const (
	// GPT3Dot5Turbo is openai's preferred gpt-3.5-turbo model.
	GPT3Dot5Turbo = openai.GPT3Dot5Turbo1106
	// GPT4 is openai's gpt-4 model.
	GPT4 = openai.GPT4

	// GPT4Turbo is openai's gpt-4 turbo preview model.
	GPT4Turbo = openai.GPT4TurboPreview
)

// AvailableModels returns a set with the available models and their
// corresponding maximum context windows.
func AvailableModels() (ret map[string]int) {
	return map[string]int{
		GPT4:          8192,
		GPT3Dot5Turbo: 16385,
		GPT4Turbo:     8192,
	}
}
