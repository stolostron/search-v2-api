package model

import (
	"encoding/json"
	"fmt"
)

func (si *SearchInput) String() string {
	siJSON, _ := json.Marshal(si)
	return fmt.Sprintf("SearchInput - %s\n", string(siJSON))
}
