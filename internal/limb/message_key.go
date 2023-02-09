package limb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/duo/octopus-qq/internal/common"
)

const SEP_EVENT_KEY = ";"

type EventKey struct {
	Seq string
	ID  string
}

func NewEventKey(seq, id int32) EventKey {
	return EventKey{
		Seq: common.Itoa(int64(seq)),
		ID:  common.Itoa(int64(id)),
	}
}

func NewPartialKey(seq int64) EventKey {
	return EventKey{
		Seq: common.Itoa(seq),
	}
}

func EventKeyFromString(str string) (*EventKey, error) {
	parts := strings.Split(str, SEP_EVENT_KEY)
	if len(parts) == 1 {
		return &EventKey{
			Seq: parts[0],
		}, nil
	} else if len(parts) == 2 {
		return &EventKey{
			Seq: parts[0],
			ID:  parts[1],
		}, nil
	} else {
		return nil, errors.New("event key format invalid")
	}
}

func (ek EventKey) IntSeq() int64 {
	number, _ := common.Atoi(ek.Seq)
	return number
}

func (ek EventKey) IntID() int64 {
	number, _ := common.Atoi(ek.ID)
	return number
}

func (mk EventKey) String() string {
	return fmt.Sprintf("%s%s%s", mk.Seq, SEP_EVENT_KEY, mk.ID)
}
