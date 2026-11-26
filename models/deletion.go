package models

import (
	"context"
	"errors"
	"time"

	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DeletedSystem identifies a soft-deleted game system. It carries both identifiers a consumer
// may have stored as a reference.
type DeletedSystem struct {
	ID        string    `bson:"_id" json:"id"`
	SystemID  string    `bson:"system_id" json:"system_id"`
	DeletedAt time.Time `bson:"deleted_at" json:"deleted_at"`
}

// DeletedResult is one page of ListDeleted output plus the total number of deleted systems.
type DeletedResult struct {
	Systems []*DeletedSystem
	Total   int
	Page    int
	PerPage int
}

// SoftDelete marks the live system matching id (document `_id` or `system_id`) deleted by
// actingUserID. Version records are kept, and the record keeps its system_id so the slug can't
// be reused. It returns (nil, nil) when no live system matches, including one already deleted,
// so the original deletion audit is never overwritten.
func SoftDelete(c context.Context, id, actingUserID string) (*DeletedSystem, error) {
	now := time.Now().UTC().Truncate(time.Millisecond) // BSON date precision
	filter := bson.D{
		{Key: "deleted_at", Value: nil},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "system_id", Value: id}},
		}},
	}
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "deleted_at", Value: now}, {Key: "deleted_by", Value: actingUserID},
		{Key: "updated_at", Value: now}, {Key: "updated_by", Value: actingUserID},
	}}}

	var deleted DeletedSystem
	err := database.Db.Collection(metaCollection).FindOneAndUpdate(c, filter, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&deleted)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &deleted, nil
}

// ListDeleted returns one page of soft-deleted systems, oldest deletion first. Page and perPage
// are clamped like List's.
func ListDeleted(c context.Context, page, perPage int) (*DeletedResult, error) {
	page, perPage = clampPage(page, perPage)
	coll := database.Db.Collection(metaCollection)
	filter := bson.D{{Key: "deleted_at", Value: bson.D{{Key: "$ne", Value: nil}}}}

	total, err := coll.CountDocuments(c, filter)
	if err != nil {
		return nil, err
	}
	cur, err := coll.Find(c, filter, options.Find().
		SetSort(bson.D{{Key: "deleted_at", Value: 1}, {Key: "_id", Value: 1}}).
		SetSkip(int64(page-1)*int64(perPage)).
		SetLimit(int64(perPage)))
	if err != nil {
		return nil, err
	}
	systems := []*DeletedSystem{}
	if err := cur.All(c, &systems); err != nil {
		return nil, err
	}
	return &DeletedResult{Systems: systems, Total: int(total), Page: page, PerPage: perPage}, nil
}
