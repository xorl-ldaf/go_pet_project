package database

import (
	"context"

	"gorm.io/gorm"
)

type gormTxKey struct{}

type TransactionRunner struct {
	db *gorm.DB
}

func NewTransactionRunner(db *gorm.DB) *TransactionRunner {
	return &TransactionRunner{db: db}
}

func (r *TransactionRunner) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ContextWithGORM(ctx, tx))
	})
}

func ContextWithGORM(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, gormTxKey{}, db)
}

func HasGORM(ctx context.Context) bool {
	tx, ok := ctx.Value(gormTxKey{}).(*gorm.DB)
	return ok && tx != nil
}

func GORMFromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(gormTxKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}

	return fallback
}
