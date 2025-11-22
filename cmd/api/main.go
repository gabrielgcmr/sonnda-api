package main

import (
	"log"

	"sonnda-api/internal/exam"
	"sonnda-api/internal/infra"
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
	infra.ConnectSupabase()
	db := infra.DB
	defer infra.CloseSupabase()

	//storage
	gcsClient, err := infra.NewGCSClient()
	if err != nil {
		log.Fatalf("falha ao criar GCS client: %v", err)
	}
	defer gcsClient.Close()

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
	patientModule := patient.NewModule(db)
	patientModule.SetupRoutes(protected)

	//Exam
	examModule := exam.NewModule(gcsClient)
	exam.RegisterRoutes(protected, examModule.Handler)

	// FUTUROS:
	// doctorHandler := doctor.Build(db)
	// doctor.Routes(api, doctorHandler)

	// examsHandler := exams.Build(db)
	// exams.Routes(api, examsHandler)

	log.Println("🚀 API running at http://localhost:8080")
	r.Run(":8080")
}
