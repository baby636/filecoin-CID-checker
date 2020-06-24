package main

import (
	"context"
	"net/http"

	"github.com/protofire/filecoin-CID-checker/internals/api/handlers"
	"github.com/protofire/filecoin-CID-checker/internals/config"
	"github.com/protofire/filecoin-CID-checker/internals/lotusprocs"
	"github.com/protofire/filecoin-CID-checker/internals/repos"

	"github.com/filecoin-project/lotus/api/client"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/toorop/gin-logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	c, err := config.LoadConfiguration()
	if err != nil {
		log.Fatal(err)
	}

	// Set client options
	clientOptions := options.Client().ApplyURI(c.Db.ConnectionString)

	// Connect to MongoDB
	mongoClient, err := mongo.Connect(context.Background(), clientOptions)

	if err != nil {
		log.WithError(err).Fatal("Failed to connect with MongoDB")
	}

	// Check the connection
	err = mongoClient.Ping(context.Background(), nil)

	if err != nil {
		log.WithError(err).Fatal("Failed to connect with MongoDB")
	}

	log.Info("Connected to MongoDB!")

	dealsRepo := repos.NewMongoDealsRepo(mongoClient, c.Db.Name)
	if err := dealsRepo.CreateIndexes(); err != nil {
		log.Fatal(err)
	}

	sectorsRepo := repos.NewMongoSectorsRepo(mongoClient, c.Db.Name)
	if err := sectorsRepo.CreateIndexes(); err != nil {
		log.Fatal(err)
	}

	lotusAPI, closer, err := client.NewFullNodeRPC(c.Lotus.RpcUrl, http.Header{})
	if err != nil {
		log.WithError(err).Fatal("Failed to connect with Lotus Node")
	}
	defer closer()

	head, err := lotusAPI.ChainHead(context.Background())
	if err != nil {
		log.WithError(err).Fatal("Failed to connect with Lotus Node")
	}
	log.Infof("Connected to Lotus API, current chain height %d", head.Height())
	lotusprocs.NewBlocksWatcher(lotusAPI).
		AddBlockEventHandler(lotusprocs.DealsProcessor(lotusAPI, dealsRepo)).
		AddBlockEventHandler(lotusprocs.SectorsProcessor(lotusAPI, dealsRepo, sectorsRepo)).
		AddBlockEventHandler(lotusprocs.MinersProcessor(lotusAPI, dealsRepo, sectorsRepo)).
		Start()

	router := gin.New()
	router.Use(ginlogrus.Logger(log.New()), gin.Recovery(), cors.Default())

	router.GET("/deals/:selector", handlers.CreateDealsHandler(dealsRepo, sectorsRepo))
	router.GET("/deals", handlers.CreateDealsHandler(dealsRepo, sectorsRepo))

	log.Fatal(router.Run(":8080"))
}
