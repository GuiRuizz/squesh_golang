package domain

import (
	"time"

	"github.com/google/uuid"
)

// PostLike registra a curtida de um usuário num post.
// Chave primária composta (UserID, PostID) garante curtida única por usuário/post.
type PostLike struct {
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	PostID    uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_like_post" json:"post_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Follow registra a relação "seguir" (FollowerID segue FollowingID).
// Chave primária composta evita seguir a mesma pessoa mais de uma vez.
type Follow struct {
	FollowerID  uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_follow_follower" json:"follower_id"`
	FollowingID uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_follow_following" json:"following_id"`
	CreatedAt   time.Time `json:"created_at"`
}
