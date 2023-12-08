package bookingratingusecase

import (
	"context"
	"paradise-booking/entities"
	"paradise-booking/modules/booking_rating/iomodel"
)

func (u *bookingRatingUsecase) MakeComment(ctx context.Context, userID int, data *iomodel.CreateBookingRatingReq) (*entities.BookingRating, error) {

	model := entities.BookingRating{
		UserId:    userID,
		BookingId: data.BookingID,
		Title:     data.Title,
		Content:   data.Content,
		Rating:    int(data.Rating),
		PlaceId:   data.PlaceID,
	}

	if _, err := u.BookingRatingSto.Create(ctx, &model); err != nil {
		return nil, err
	}

	return &model, nil
}
