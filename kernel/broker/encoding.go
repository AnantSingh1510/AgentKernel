package broker

import (
	"encoding/json"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

func marshalEvent(e *types.Event) ([]byte, error) {
	return json.Marshal(e)
}

func unmarshalEvent(data []byte) (*types.Event, error) {
	var e types.Event
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
