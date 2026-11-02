package composecore_test

import (
	"testing"

	"github.com/ousiassllc/hitomado/internal/app/composecore/composetest"
)

// composetest のフィクスチャ・補助関数の短い別名。
type composeFixture = composetest.Fixture

const (
	threadID = composetest.ThreadID
	aliceMsg = composetest.AliceMsg
)

var (
	addr         = composetest.Addr
	toContent    = composetest.ToContent
	contentOf    = composetest.ContentOf
	requireCode  = composetest.RequireCode
	requireError = composetest.RequireError
)

func newComposeFixture(t *testing.T) *composeFixture { return composetest.New(t) }
