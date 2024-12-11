package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	pb "campeoneslol-grpc/proto" // Importa el paquete generado

	_ "github.com/denisenkom/go-mssqldb"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

var db *sql.DB

type server struct {
	pb.UnimplementedCampeonServiceServer // Implementa el servicio generado
}

func (s *server) GetCampeonInfo(ctx context.Context, req *pb.CampeonRequest) (*pb.CampeonResponse, error) {
	var name, ptype, tier string

	query := "SELECT * FROM champsprd.campeones WHERE Name LIKE @Name"
	row := db.QueryRowContext(ctx, query, sql.Named("Name", "%"+req.Name+"%"))

	err := row.Scan(&name, &ptype, &tier)
	if err != nil {
		if err == sql.ErrNoRows {
			return &pb.CampeonResponse{
				Name: "Not Found",
				Type: "Not Found",
				Tier: "D",
			}, nil
		}
		return nil, err
	}

	return &pb.CampeonResponse{
		Name: name,
		Type: ptype,
		Tier: tier,
	}, nil
}

func (s *server) GetCampeonList(req *pb.Empty, stream pb.CampeonService_GetCampeonListServer) error {
	query := "select * from champsprd.campeones"
	rows, err := db.Query(query)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, ptype, tier string

		if err := rows.Scan(
			&name,
			&ptype,
			&tier,
		); err != nil {
			log.Panic(err)
			return err
		}

		if err := stream.Send(&pb.CampeonResponse{
			Name: name,
			Type: ptype,
			Tier: tier,
		}); err != nil {
			log.Panic(err)
			return err
		}
	}
	return nil
}

func (s *server) AddCampeones(stream pb.CampeonService_AddCampeonesServer) error {
	var count int32

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.AddCampeonResponse{
				Count: count,
			})
		}

		if err != nil {
			log.Panic(err)
			return err
		}

		query := "insert into champsprd.campeones (Name, Type, Tier) values (@Name, @Type, @Tier)"
		_, err = db.Exec(query, sql.Named("Name", req.Name), sql.Named("Type", req.Type), sql.Named("Tier", req.Tier))

		if err != nil {
			log.Panic(err)
			return err
		}

		count++
		log.Println("Added", req.Name)
	}
}

func (s *server) GetCampeonByType(stream pb.CampeonService_GetCampeonByTypeServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			log.Println("End of stream")
			return nil
		}
		if err != nil {
			log.Panic(err)
			return err
		}

		query := "SELECT * from champsprd.campeones WHERE LOWER(Type) = LOWER (@Type)"
		rows, err := db.Query(query, sql.Named("Type", req.Type))
		if err != nil {
			log.Panic(err)
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var name, ptype, tier string

			if err := rows.Scan(&name, &ptype, &tier); err != nil {
				log.Panic(err)
				return err
			}

			if err := stream.Send(&pb.CampeonResponse{
				Name: name,
				Type: ptype,
				Tier: tier,
			}); err != nil {
				log.Panic(err)
				return err
			}
		}
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	s := os.Getenv("DB_SERVER")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	database := os.Getenv("DB_NAME")
	port := os.Getenv("DB_PORT")

	connString := fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", user, password, s, port, database)
	db, err = sql.Open("sqlserver", connString)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	log.Println("Connected to database")

	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		panic(err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterCampeonServiceServer(grpcServer, &server{}) // Registra el servicio

	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})
		log.Println("Starting health check server on port 8080")
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	log.Println("Starting server on port :50051")
	if err := grpcServer.Serve(listener); err != nil {
		panic(err)
	}
}
