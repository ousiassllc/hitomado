package platform

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// files-dropped のペイロードは、フロント（frontend/src/lib/dropFiles.ts の droppedPaths）が読む
// `{ "files": [...] }` の形で届ける。キー名が変わるとドロップしても何も添付されなくなる。
func TestFilesDroppedPayloadJSONUsesFilesKey(t *testing.T) {
	b, err := json.Marshal(FilesDroppedPayload{Files: []string{"/tmp/a.txt", "/tmp/b.txt"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"files":["/tmp/a.txt","/tmp/b.txt"]}`, string(b))
}
