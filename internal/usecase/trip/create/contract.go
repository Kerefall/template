//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/model"
)

type (
	tripRepository interface {
		Save(ctx context.Context, t trip.Trip) error
		AddStatusHistory(ctx context.Context, change model.StatusChange) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	metrics interface {
		TripCreated(ctx context.Context)
	}
)
