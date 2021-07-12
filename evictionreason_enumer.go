package ttl

import (
	"fmt"
)

const evictionReasonName = "RemovedEvictedSizeExpiredClosed"

var evictionReasonIndex = [...]uint8{0, 7, 18, 25, 31}

func (i EvictionReason) String() string {
	if i < 0 || i >= EvictionReason(len(evictionReasonIndex)-1) {
		return fmt.Sprintf("EvictionReason(%d)", i)
	}
	return evictionReasonName[evictionReasonIndex[i]:evictionReasonIndex[i+1]]
}

var evictionReasonValues = []EvictionReason{0, 1, 2, 3}

var evictionReasonNameToValueMap = map[string]EvictionReason{
	evictionReasonName[0:7]:   0,
	evictionReasonName[7:18]:  1,
	evictionReasonName[18:25]: 2,
	evictionReasonName[25:31]: 3,
}

// EvictionReasonString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func EvictionReasonString(s string) (EvictionReason, error) {
	if val, ok := evictionReasonNameToValueMap[s]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to EvictionReason values", s)
}

// EvictionReasonValues returns all values of the enum
func EvictionReasonValues() []EvictionReason {
	return evictionReasonValues
}

// IsAEvictionReason returns "true" if the value is listed in the enum definition. "false" otherwise
func (i EvictionReason) IsAEvictionReason() bool {
	for _, v := range evictionReasonValues {
		if i == v {
			return true
		}
	}
	return false
}
