//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/model"
)

type usecase interface {
	Create(ctx context.Context, in model.CreateTripInput) (trip.Trip, error)
}
