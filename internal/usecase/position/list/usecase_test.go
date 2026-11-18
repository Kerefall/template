package list_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/usecase/position/list"
)

func newUseCase(t *testing.T) (*list.UseCase, *MocktripRepository, *MockpositionRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	positions := NewMockpositionRepository(ctrl)

	return list.New(trips, positions), trips, positions
}

func TestList_ReturnsPositions(t *testing.T) {
	uc, trips, positions := newUseCase(t)
	id := uuid.New()
	want := []position.Position{{ID: 1, TripID: id}, {ID: 2, TripID: id}}

	trips.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{ID: id}, nil)
	positions.EXPECT().ListPositions(gomock.Any(), id).Return(want, nil)

	got, err := uc.List(context.Background(), id)

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestList_TripWithoutPositionsHasEmptyRoute(t *testing.T) {
	uc, trips, positions := newUseCase(t)
	id := uuid.New()

	trips.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{ID: id}, nil)
	positions.EXPECT().ListPositions(gomock.Any(), id).Return([]position.Position{}, nil)

	got, err := uc.List(context.Background(), id)

	require.NoError(t, err)
	require.Empty(t, got)
}

func TestList_UnknownTrip(t *testing.T) {
	uc, trips, _ := newUseCase(t)
	id := uuid.New()

	trips.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)
	// ListPositions не вызывается: ожидания нет.

	_, err := uc.List(context.Background(), id)

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}
