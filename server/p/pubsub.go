package p

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsub"
)

func PublishNewMacroMessage(macroName string) {
	publishNewMacroMessage(macroName)
}

func publishNewMacroMessage(macroName string) error {
	ctx := context.Background()

	client, err := pubsub.NewClient(ctx, "github-macros")
	if err != nil {
		return fmt.Errorf("failed to create pubsub client: %v", err)
	}

	m := &pubsub.Message{
		Data: []byte(macroName),
	}

	if _, err := client.Topic("macro_was_added").Publish(ctx, m).Get(ctx); err != nil {
		return fmt.Errorf("failed to publish: %v", err)
	}

	return nil
}
