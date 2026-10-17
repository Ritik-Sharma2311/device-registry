// Package mongodb is a MongoDB-backed device.Store.
package mongodb

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
)

// doc is the stored shape; _id carries the device ID.
type doc struct {
	ID        string    `bson:"_id"`
	Name      string    `bson:"name"`
	Kind      string    `bson:"kind"`
	Address   string    `bson:"address"`
	Status    string    `bson:"status"`
	Version   int64     `bson:"version"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func fromDevice(d device.Device) doc {
	return doc{d.ID, d.Name, d.Kind, d.Address, d.Status, d.Version, d.CreatedAt, d.UpdatedAt}
}

func (d doc) toDevice() device.Device {
	return device.Device{
		ID: d.ID, Name: d.Name, Kind: d.Kind, Address: d.Address, Status: d.Status,
		Version: d.Version, CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type Store struct {
	client *mongo.Client
	coll   *mongo.Collection
}

// Open connects to uri, uses the named database and ensures indexes exist.
func Open(ctx context.Context, uri, database string) (*Store, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	coll := client.Database(database).Collection("devices")
	_, err = coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "kind", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
	})
	if err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	return &Store{client: client, coll: coll}, nil
}

func (s *Store) Close() { _ = s.client.Disconnect(context.Background()) }

func (s *Store) Create(ctx context.Context, d device.Device) error {
	_, err := s.coll.InsertOne(ctx, fromDevice(d))
	if mongo.IsDuplicateKeyError(err) {
		return device.ErrConflict
	}
	return err
}

func (s *Store) Get(ctx context.Context, id string) (device.Device, error) {
	var d doc
	err := s.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return device.Device{}, device.ErrNotFound
	}
	if err != nil {
		return device.Device{}, err
	}
	return d.toDevice(), nil
}

func (s *Store) List(ctx context.Context, f device.Filter) ([]device.Device, error) {
	filter := bson.M{}
	if f.Kind != "" {
		filter["kind"] = f.Kind
	}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.AfterID != "" {
		filter["_id"] = bson.M{"$gt": f.AfterID}
	}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
	if f.Limit > 0 {
		opts.SetLimit(int64(f.Limit))
	}
	cur, err := s.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []doc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]device.Device, len(docs))
	for i, d := range docs {
		out[i] = d.toDevice()
	}
	return out, nil
}

// Update is a compare-and-swap on version in one UpdateOne call.
func (s *Store) Update(ctx context.Context, in device.Device) (device.Device, error) {
	var d doc
	err := s.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": in.ID, "version": in.Version},
		bson.M{
			"$set": bson.M{"name": in.Name, "kind": in.Kind, "address": in.Address, "status": in.Status, "updated_at": in.UpdatedAt},
			"$inc": bson.M{"version": 1},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&d)
	switch {
	case err == nil:
		return d.toDevice(), nil
	case mongo.IsDuplicateKeyError(err):
		return device.Device{}, device.ErrConflict
	case errors.Is(err, mongo.ErrNoDocuments):
		n, cerr := s.coll.CountDocuments(ctx, bson.M{"_id": in.ID})
		if cerr != nil {
			return device.Device{}, cerr
		}
		if n > 0 {
			return device.Device{}, device.ErrVersionConflict
		}
		return device.Device{}, device.ErrNotFound
	default:
		return device.Device{}, err
	}
}

func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.coll.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return device.ErrNotFound
	}
	return nil
}
