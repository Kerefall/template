package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/usecase/trip/get"
)

func TestGet(t *testing.T) {
	want := trip.Trip{ID: uuid.New(), Status: trip.StatusActive, Price: 1450}

	cases := []struct {
		name    string
		repoRes trip.Trip
		repoErr error
		wantErr error
	}{
		{name: "found", repoRes: want},
		{name: "not found", repoErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "repository failure", repoErr: errors.New("connection reset"), wantErr: errors.New("connection reset")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewMocktripRepository(gomock.NewController(t))
			repo.EXPECT().Get(gomock.Any(), want.ID).Return(tc.repoRes, tc.repoErr)

			got, err := get.New(repo).Get(context.Background(), want.ID)

			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestGet_NotFoundIsSentinel(t *testing.T) {
	repo := NewMocktripRepository(gomock.NewController(t))
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).Return(trip.Trip{}, trip.ErrTripNotFound)

	_, err := get.New(repo).Get(context.Background(), uuid.New())

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}
