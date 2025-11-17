package main

import (
	//"context"
	"log"

	"sonnda-api/internal/database"
	//"sonnda-api/internal/exam"
	"sonnda-api/internal/middleware"
	"sonnda-api/internal/patient"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	//context
	//ctx := context.Background()

	//carregar .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env não carregado automaticamente")
	}

	//conectar db
	database.Connect()
	db := database.DB
	defer database.Close()

	//montar o gin e rotas
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 🌐 Aplica o middleware de CORS
	r.Use(middleware.SetupCors())

	middleware.InitSupabaseAuth()

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
	//AuthSupabase
	protected := apiV1.Group("")
	protected.Use(middleware.SupabaseAuth())

	//Patient
	patientHandler := patient.Build(db)
	patient.Routes(protected, patientHandler)

	//Exam
	//examModule, _ := exam.NewModule(ctx, "sonnda.firebasestorage.app")
	//exam.RegisterRoutes(protected, examModule.Handler)

	// FUTUROS:
	// doctorHandler := doctor.Build(db)
	// doctor.Routes(api, doctorHandler)

	// examsHandler := exams.Build(db)
	// exams.Routes(api, examsHandler)

	log.Println("🚀 API running at http://localhost:8080")
	r.Run(":8080")
}
