package output

import (
	"encoding/json"
	"io"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

func RenderJSON(w io.Writer, inv types.Inventory) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(inv)
}
