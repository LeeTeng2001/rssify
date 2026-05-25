package ai

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type Message struct {
	Role    string
	Content string
}

type Client struct {
	inner openai.Client
	model string
}

func New(baseURL, apiKey, model string) *Client {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &Client{
		inner: openai.NewClient(opts...),
		model: model,
	}
}

func (c *Client) Complete(ctx context.Context, msgs []Message) (string, error) {
	params := openai.ChatCompletionNewParams{
		Model:       c.model,
		Temperature: openai.Float(0.2),
	}
	for _, m := range msgs {
		switch m.Role {
		case "system":
			params.Messages = append(params.Messages, openai.SystemMessage(m.Content))
		case "assistant":
			params.Messages = append(params.Messages, openai.AssistantMessage(m.Content))
		default:
			params.Messages = append(params.Messages, openai.UserMessage(m.Content))
		}
	}
	completion, err := c.inner.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}
	if len(completion.Choices) == 0 {
		return "", nil
	}
	return completion.Choices[0].Message.Content, nil
}