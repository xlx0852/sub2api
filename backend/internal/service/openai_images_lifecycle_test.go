package service

import "testing"

func TestForwardImagesRejectsMissingAccount(t *testing.T) {
	_, err := (&OpenAIGatewayService{}).ForwardImages(nil, nil, nil, nil, &OpenAIImagesRequest{}, "")
	if err == nil || err.Error() != "images account is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}
