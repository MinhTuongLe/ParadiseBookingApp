package bookingratinghandler

import (
	"context"
	"paradise-booking/entities"
	"paradise-booking/modules/booking_rating/iomodel"
)

type placeRatingUseCase interface {
	MakeComment(ctx context.Context, userID int, data *iomodel.CreateBookingRatingReq) (*entities.BookingRating, error)
}

type bookingratinghandler struct {
	placeRatingUC placeRatingUseCase
}

func Newbookingratinghandler(placeRatingUseCase placeRatingUseCase) *bookingratinghandler {
	return &bookingratinghandler{placeRatingUC: placeRatingUseCase}
}