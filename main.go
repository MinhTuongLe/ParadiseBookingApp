package main

import (
	"log"
	"net/http"
	cmdworker "paradise-booking/cmd/worker"
	"paradise-booking/config"
	"paradise-booking/constant"
	accounthandler "paradise-booking/modules/account/handler"
	accountstorage "paradise-booking/modules/account/storage"
	accountusecase "paradise-booking/modules/account/usecase"
	bookinghandler "paradise-booking/modules/booking/handler"
	bookingstorage "paradise-booking/modules/booking/storage"
	bookingusecase "paradise-booking/modules/booking/usecase"
	bookingdetailstorage "paradise-booking/modules/booking_detail/storage"
	"paradise-booking/modules/middleware"
	placehandler "paradise-booking/modules/place/handler"
	placestorage "paradise-booking/modules/place/storage"
	placeusecase "paradise-booking/modules/place/usecase"
	placewishlisthandler "paradise-booking/modules/place_wishlist/handler"
	placewishliststorage "paradise-booking/modules/place_wishlist/storage"
	placewishlistusecase "paradise-booking/modules/place_wishlist/usecase"
	uploadhandler "paradise-booking/modules/upload/handler"
	uploadusecase "paradise-booking/modules/upload/usecase"
	verifyemailshanlder "paradise-booking/modules/verify_emails/handler"
	verifyemailsstorage "paradise-booking/modules/verify_emails/storage"
	verifyemailsusecase "paradise-booking/modules/verify_emails/usecase"
	wishlisthandler "paradise-booking/modules/wishlist/handler"
	wishliststorage "paradise-booking/modules/wishlist/storage"
	wishlistusecase "paradise-booking/modules/wishlist/usecase"
	"paradise-booking/provider/cache"
	mysqlprovider "paradise-booking/provider/mysql"
	redisprovider "paradise-booking/provider/redis"
	s3provider "paradise-booking/provider/s3"
	"paradise-booking/utils"
	"paradise-booking/worker"
	"sync"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalln("Get config error", err)
		return
	}

	// Declare DB
	db, err := mysqlprovider.NewMySQL(cfg)
	if err != nil {
		log.Fatalln("Can not connect mysql: ", err)
	}

	utils.RunDBMigration(cfg)

	// Declare redis
	redis, err := redisprovider.NewRedisClient(cfg)
	if err != nil {
		log.Fatalln("Can not connect redis: ", err)
	}

	// declare redis client for asynq
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Host + ":" + cfg.Redis.Port,
		Password: cfg.Redis.Password,
	}

	// declare task distributor
	taskDistributor := worker.NewRedisTaskDistributor(&redisOpt)

	// declare dependencies account
	accountSto := accountstorage.NewAccountStorage(db)
	accountCache := cache.NewAuthUserCache(accountSto, cache.NewRedisCache(redis))

	// declare verify email usecase
	verifyEmailsSto := verifyemailsstorage.NewVerifyEmailsStorage(db)
	verifyEmailsUseCase := verifyemailsusecase.NewVerifyEmailsUseCase(verifyEmailsSto, accountSto)
	verifyEmailsHdl := verifyemailshanlder.NewVerifyEmailsHandler(verifyEmailsUseCase)

	accountUseCase := accountusecase.NewUserUseCase(cfg, accountSto, verifyEmailsUseCase, taskDistributor)
	accountHdl := accounthandler.NewAccountHandler(cfg, accountUseCase)

	// declare dependencies

	// prepare for place
	placeSto := placestorage.NewPlaceStorage(db)
	placeUseCase := placeusecase.NewPlaceUseCase(cfg, placeSto, accountSto)
	placeHdl := placehandler.NewPlaceHandler(placeUseCase)

	// prepare for booking detail
	bookingDetailSto := bookingdetailstorage.NewBookingDetailStorage(db)

	// prepare for booking
	bookingSto := bookingstorage.NewBookingStorage(db)
	bookingUseCase := bookingusecase.NewBookingUseCase(bookingSto, bookingDetailSto, cfg, taskDistributor, accountSto, placeSto)
	bookingHdl := bookinghandler.NewBookingHandler(bookingUseCase)

	// prepare for wish list
	wishListSto := wishliststorage.NewWishListStorage(db)
	wishListUseCase := wishlistusecase.NewWishListUseCase(wishListSto)
	wishListHdl := wishlisthandler.NewWishListHandler(wishListUseCase)

	// prepare place wish list
	placeWishListSto := placewishliststorage.NewPlaceWishListStorage(db)
	placeWishListUseCase := placewishlistusecase.NewPlaceWishListUseCase(placeWishListSto, placeSto)
	placeWishListHdl := placewishlisthandler.NewPlaceWishListHandler(placeWishListUseCase)

	// upload file to s3
	s3Provider := s3provider.NewS3Provider(cfg)
	uploadUC := uploadusecase.NewUploadUseCase(cfg, s3Provider)
	uploadHdl := uploadhandler.NewUploadHandler(cfg, uploadUC)

	// run task processor
	wg := new(sync.WaitGroup)

	wg.Add(1)
	go func() {
		defer wg.Done()
		cmdworker.RunTaskProcessor(&redisOpt, accountSto, cfg, verifyEmailsUseCase, bookingSto)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		cmdworker.RunTaskScheduler(&redisOpt, cfg)
	}()

	router := gin.Default()

	// config CORS
	configCORS := setupCors()
	router.Use(cors.New(configCORS))

	middlewares := middleware.NewMiddlewareManager(cfg, accountCache)
	router.Use(middlewares.Recover())

	v1 := router.Group("/api/v1")

	// health check
	v1.GET("/", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"Hello": "World"})
	})
	v1.GET("/healthchecker", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// User
	v1.POST("/register", accountHdl.RegisterAccount())
	v1.POST("/login", accountHdl.LoginAccount())
	v1.PATCH("/account/:id", accountHdl.UpdatePersonalInfoAccountById())
	v1.GET("/profile", accountHdl.GetAccountByEmail())
	v1.GET("/profile/:id", accountHdl.GetAccountByID())
	v1.GET("/accounts", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.AdminRole), accountHdl.GetAllAccountUserAndVendor())
	v1.PATCH("/account/role/:id", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.AdminRole), accountHdl.UpdateAccountRoleByID())
	v1.POST("/change/password", middlewares.RequiredAuth(), accountHdl.ChangePassword())
	v1.POST("/change/status", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.AdminRole), accountHdl.ChangeStatusAccount())
	v1.POST("/forgot/password", accountHdl.ForgotPassword())
	v1.POST("/reset/password", accountHdl.ResetPassword())

	// Place
	v1.POST("/places", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.VendorRole), placeHdl.CreatePlace())
	v1.PUT("/places", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.VendorRole), placeHdl.UpdatePlace())
	v1.GET("/places/:id", placeHdl.GetPlaceByID())
	v1.GET("/places/owner", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.VendorRole), placeHdl.ListPlaceByVendor())
	v1.GET("/places/owner/:vendor_id", placeHdl.ListPlaceByVendorID())
	v1.DELETE("/places", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.VendorRole), placeHdl.DeletePlaceByID())
	v1.GET("/places", placeHdl.ListAllPlace())

	// booking
	v1.POST("/bookings", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), bookingHdl.CreateBooking())
	v1.GET("/confirm_booking", bookingHdl.UpdateStatusBooking())
	v1.POST("/booking_list", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), bookingHdl.ListBooking())
	v1.GET("/bookings/:id", middlewares.RequiredAuth(), bookingHdl.GetBookingByID())
	v1.GET("/bookings", middlewares.RequiredAuth(), bookingHdl.GetBookingByPlaceID())
	v1.GET("/bookings_list/manage_reservation", middlewares.RequiredAuth(), bookingHdl.ListBookingNotReservation())
	v1.DELETE("/bookings/:id", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), bookingHdl.DeleteBookingByID())
	v1.POST("/cancel_booking", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), bookingHdl.CancelBookingByID())

	// wish list
	v1.POST("/wish_lists", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), wishListHdl.CreateWishList())
	v1.GET("/wish_lists/:id", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), wishListHdl.GetWishListByID())
	v1.GET("/wish_lists/user/:user_id", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), wishListHdl.GetWishListByUserID())

	// place wish list
	v1.POST("/place_wish_lists", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), placeWishListHdl.CreatePlaceWishList())
	v1.DELETE("/place_wish_lists/:place_id/:wishlist_id", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), placeWishListHdl.DeletePlaceWishList())
	v1.GET("/place_wish_lists/place", middlewares.RequiredAuth(), middlewares.RequiredRoles(constant.UserRole, constant.VendorRole), placeWishListHdl.ListPlaceByWishListID())
	// verify email
	v1.GET("/verify_email", verifyEmailsHdl.CheckVerifyCodeIsMatching())

	// verify reset code password
	v1.GET("/verify_reset_password", verifyEmailsHdl.CheckResetCodePasswordIsMatching())

	// upload file to s3
	v1.POST("/upload", middlewares.RequiredAuth(), uploadHdl.UploadFile())

	// google login
	//v1.GET("/google/login")
	router.Run(":" + cfg.App.Port)
	wg.Wait()

}

func setupCors() cors.Config {
	configCORS := cors.DefaultConfig()
	configCORS.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	configCORS.AllowHeaders = []string{"Origin", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization"}
	//configCORS.AllowOrigins = []string{"http://localhost:3000"}
	configCORS.AllowAllOrigins = true

	return configCORS
}
