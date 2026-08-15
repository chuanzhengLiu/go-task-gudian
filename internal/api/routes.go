package api

import (
	"ancient-texts-backend/internal/middleware"
	"ancient-texts-backend/internal/model"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	r.Use(middleware.SecurityHeaders())

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:8080", "http://localhost:5173", "http://localhost:3000", "http://localhost"},
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-CSRF-Token"},
		ExposeHeaders:    []string{"Content-Length", "Content-Disposition", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.Use(middleware.CSRF())

	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})

		auth := api.Group("/auth")
		{
			auth.POST("/register", middleware.RateLimit(5, time.Minute), Register)
			auth.POST("/login", middleware.RateLimit(5, time.Minute), Login)
			auth.POST("/refresh", RefreshToken)
			auth.POST("/logout", middleware.AuthRequired(), Logout)
			auth.GET("/me", middleware.AuthRequired(), GetCurrentUser)
			auth.POST("/change-password", middleware.AuthRequired(), ChangePassword)

			auth.GET("/sessions", middleware.AuthRequired(), GetSessions)
			auth.DELETE("/sessions/:id", middleware.AuthRequired(), RevokeSession)

			auth.GET("/totp/secret", middleware.AuthRequired(), GetTOTPSecret)
			auth.POST("/totp/enable", middleware.AuthRequired(), EnableTOTP)
			auth.POST("/totp/disable", middleware.AuthRequired(), DisableTOTP)
		}

		admin := api.Group("/admin")
		admin.Use(middleware.AuthRequired(), middleware.AdminOnly())
		{
			admin.GET("/institutions", ListInstitutions)
			admin.PUT("/institutions/:id/approve", ApproveInstitution)

			admin.POST("/variants", middleware.RateLimit(20, time.Minute), CreateVariant)
			admin.GET("/variants", ListVariants)
			admin.PUT("/variants/:id", UpdateVariant)
			admin.DELETE("/variants/:id", DeleteVariant)
			admin.POST("/variants/import", BatchImportVariants)

			admin.GET("/audit-logs", middleware.RateLimit(100, time.Minute), GetAuditLogs)

			admin.PUT("/users/:id", UpdateUser)
			admin.DELETE("/users/:id", DeleteUser)
		}

		institutions := api.Group("/institutions")
		institutions.Use(middleware.AuthRequired())
		{
			institutions.POST("", middleware.RateLimit(3, time.Hour), RegisterInstitution)
			institutions.GET("", ListInstitutions)
			institutions.GET("/:id", GetInstitution)
			institutions.PUT("/:id", middleware.InstAdminOrHigher(), UpdateInstitution)
			institutions.GET("/:id/users", ListInstitutionUsers)
			institutions.POST("/:id/users", middleware.InstAdminOrHigher(), InviteUser)
		}

		projects := api.Group("/projects")
		projects.Use(middleware.AuthRequired())
		{
			projects.POST("", middleware.InstAdminOrHigher(), CreateProject)
			projects.GET("", ListProjects)
			projects.GET("/:id", GetProject)
			projects.PUT("/:id", middleware.InstAdminOrHigher(), UpdateProject)
			projects.DELETE("/:id", middleware.InstAdminOrHigher(), DeleteProject)
			projects.POST("/:id/assign", middleware.InstAdminOrHigher(), AssignPages)
			projects.GET("/:id/board", GetProjectBoard)
			projects.GET("/:id/stats", ExportStats)

			uploads := projects.Group("/:id")
			uploads.Use(middleware.RateLimit(10, time.Minute))
			{
				uploads.POST("/upload-image", middleware.InstAdminOrHigher(), UploadImage)
				uploads.POST("/upload-ocr", middleware.InstAdminOrHigher(), UploadOCR)
				uploads.POST("/batch-upload", middleware.InstAdminOrHigher(), BatchUploadImages)
				uploads.POST("/upload-emendations", middleware.InstAdminOrHigher(), UploadEmendations)
			}

			variants := projects.Group("/:id/variants")
			{
				variants.GET("", middleware.ProofreaderOrHigher(), ListCustomVariants)
				variants.POST("", middleware.ProofreaderOrHigher(), CreateCustomVariant)
				variants.DELETE("/:variant_id", middleware.ProofreaderOrHigher(), DeleteCustomVariant)
			}

			compare := projects.Group("/:id/compare")
			{
				compare.POST("", middleware.ProofreaderOrHigher(), CompareVersions)
				compare.GET("", ListCompareTasks)
				compare.GET("/:task_id", GetCompareTask)
			}

			export := projects.Group("/:id/export")
			{
				export.GET("/text", middleware.RoleRequired(model.RoleAdmin, model.RoleInstAdmin, model.RoleTypesetter), ExportText)
				export.GET("/tei", middleware.RoleRequired(model.RoleAdmin, model.RoleInstAdmin, model.RoleTypesetter), ExportTEI)
				export.GET("/excel", middleware.RoleRequired(model.RoleAdmin, model.RoleInstAdmin, model.RoleTypesetter), ExportExcel)
				export.GET("/pdf", middleware.RoleRequired(model.RoleAdmin, model.RoleInstAdmin, model.RoleTypesetter), ExportPDF)
			}
		}

		pages := api.Group("/pages")
		pages.Use(middleware.AuthRequired())
		{
			pages.POST("", middleware.InstAdminOrHigher(), CreatePage)
			pages.POST("/batch", middleware.InstAdminOrHigher(), BatchCreatePages)
			pages.GET("/:id", GetPage)
			pages.PUT("/:id", middleware.ProofreaderOrHigher(), UpdatePage)
			pages.POST("/:id/lock", middleware.ProofreaderOrHigher(), LockPage)
			pages.POST("/:id/unlock", middleware.ProofreaderOrHigher(), UnlockPage)
			pages.POST("/:id/submit", middleware.ProofreaderOrHigher(), SubmitForReview)
			pages.POST("/:id/autosave", middleware.ProofreaderOrHigher(), AutoSave)

			corrections := pages.Group("/:id/corrections")
			{
				corrections.GET("", GetPageCorrections)
				corrections.POST("", middleware.ProofreaderOrHigher(), CreateCorrection)
				corrections.PUT("/:cid", middleware.ProofreaderOrHigher(), UpdateCorrection)
				corrections.DELETE("/:cid", middleware.ProofreaderOrHigher(), DeleteCorrection)
			}

			reviews := pages.Group("/:id/reviews")
			{
				reviews.GET("", GetPageReviews)
				reviews.POST("", middleware.RoleRequired(model.RoleAdmin, model.RoleInstAdmin, model.RoleProofreader2), SubmitReview)
			}

			variantDetect := pages.Group("/:id")
			{
				variantDetect.POST("/detect-variants", middleware.ProofreaderOrHigher(), DetectVariants)
			}
		}

		review := api.Group("/reviews")
		review.Use(middleware.AuthRequired(), middleware.RoleRequired(model.RoleAdmin, model.RoleInstAdmin, model.RoleProofreader2))
		{
			review.GET("", ListReviewTasks)
			review.PUT("/:id", UpdateReview)
		}

		api.GET("/variants/stats", middleware.AuthRequired(), GetVariantStats)

		api.GET("/images/*path", GetImage)
		api.GET("/tiles/*path", GetTile)
	}
}
