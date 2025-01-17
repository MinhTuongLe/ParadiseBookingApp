package bookinghandler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (hdl *bookingHandler) UpdateStatusBooking() gin.HandlerFunc {
	return func(c *gin.Context) {
		bookingID, _ := c.GetQuery("booking_id")
		status, _ := c.GetQuery("status")

		bookingId, _ := strconv.Atoi(bookingID)
		statusInt, _ := strconv.Atoi(status)

		err := hdl.bookingUC.UpdateStatusBooking(c.Request.Context(), bookingId, statusInt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": true})
	}
}
