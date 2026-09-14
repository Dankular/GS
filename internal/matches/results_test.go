package matches

import (
	"encoding/json"
	"testing"
)

func TestResultValidationCanonicalizesAndBindsDigest(t *testing.T) {
	submission := ResultSubmission{MatchID: "match-1", Sequence: 1, Payload: json.RawMessage(` { "score": 10 } `)}
	canonical, digest, err := submission.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != `{"score":10}` || digest != Digest(canonical) {
		t.Fatalf("unexpected canonical result: %s %s", canonical, digest)
	}
	submission.PayloadDigest = "sha256:wrong"
	if _, _, err := submission.Validate(); err != ErrResultDigestMismatch {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

func TestResultValidationRejectsInvalidPayload(t *testing.T) {
	if _, _, err := (ResultSubmission{MatchID: "m", Sequence: 0, Payload: json.RawMessage(`{}`)}).Validate(); err == nil {
		t.Fatal("expected sequence rejection")
	}
	if _, _, err := (ResultSubmission{MatchID: "m", Sequence: 1, Payload: json.RawMessage(`not-json`)}).Validate(); err == nil {
		t.Fatal("expected JSON rejection")
	}
}
