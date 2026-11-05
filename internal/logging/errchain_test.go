package logging

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

type loopErr struct{}

func (e *loopErr) Error() string { return "loop" }
func (e *loopErr) Unwrap() error { return e }

func TestErrorChainListsTypesWithoutMessages(t *testing.T) {
	inner := &url.Error{Op: "Post", URL: "https://example.invalid/token?code=SECRETCODE", Err: errors.New("boom")}
	err := fmt.Errorf("oauth: 失敗: %w", inner)

	got := ErrorChain(err)

	assert.Equal(t, "*fmt.wrapError > *url.Error > *errors.errorString", got)
	assert.NotContains(t, got, "SECRETCODE")
}

func TestErrorChainNil(t *testing.T) {
	assert.Empty(t, ErrorChain(nil))
}

func TestErrorChainFoldsJoinedBranches(t *testing.T) {
	err := fmt.Errorf("wrap: %w", errors.Join(errors.New("a"), fmt.Errorf("b: %w", errors.New("c"))))

	assert.Equal(t, "*fmt.wrapError > *errors.joinError > [*errors.errorString|*fmt.wrapError > *errors.errorString]", ErrorChain(err))
}

func TestErrorChainStopsOnCycle(t *testing.T) {
	got := ErrorChain(&loopErr{})

	assert.Len(t, got, len("*logging.loopErr")*maxChainDepth+len(" > ")*(maxChainDepth-1))
}
