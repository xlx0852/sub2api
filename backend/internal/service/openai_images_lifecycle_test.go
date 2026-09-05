package service

import (
	"context"
	"testing"
)

func TestForwardImagesRejectsMissingAccount(t *testing.T) {
	_, err := (&OpenAIGatewayService{}).ForwardImages(context.Background(), nil, nil, nil, &OpenAIImagesRequest{}, "")
	if err == nil || err.Error() != "images account is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}
