//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package save

import (
	"context"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/model"
)

type usecase interface {
	Save(ctx context.Context, in model.SavePositionInput) (position.Position, error)
}
