package main

import (
	"log"
	"os"

	"time"

	"sonnda-api/internal/auth"
	"sonnda-api/internal/core/jwt"
	"sonnda-api/internal/core/model"
	"sonnda-api/internal/database"
	"sonnda-api/internal/exam"
	"sonnda-api/internal/middleware"
	"sonnda-api/internal/patient"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	//carregar .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env não carregado automaticamente")
	}

	//conectar db
	database.Connect()
	db := database.DB

	//montar o gin e rotas
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 🌐 Aplica o middleware de CORS
	r.Use(middleware.SetupCors())

	//config
	jwtMgr := jwt.NewJWTManager(
		os.Getenv("JWT_SECRET"),
		"sonnda-api",
		24*time.Hour,
	)

	//routes
	apiV1 := r.Group("/api/v1")

	//rotas de saúde
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": "1.0.0",
		})
	})

	//Modules
	//Auth
	authHandler := auth.Build(db, jwtMgr)
	auth.Routes(apiV1, authHandler, jwtMgr)

	//Patient
	patientHandler := patient.Build(db)
	patient.Routes(apiV1, patientHandler)

	//Exam
	examModule, _ := exam.NewModule(ctx, "sonnda.firebasestorage.app")
	exam.RegisterRoutes(apiV1, examModule.Handler)

	// FUTUROS:
	// doctorHandler := doctor.Build(db)
	// doctor.Routes(api, doctorHandler)

	// examsHandler := exams.Build(db)
	// exams.Routes(api, examsHandler)

	//migrations
	if err := db.AutoMigrate(
		&model.User{},
		&model.Patient{},
	); err != nil {
		log.Fatalf("Erro ao migrar tabela users: %v", err)
	}

	log.Println("🚀 API running at http://localhost:8080")
	r.Run(":8080")
}
