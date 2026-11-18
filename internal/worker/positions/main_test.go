package positions

import (
	"testing"

	"go.uber.org/goleak"
)

// После всех тестов не должно остаться ни одной горутины компонента.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
