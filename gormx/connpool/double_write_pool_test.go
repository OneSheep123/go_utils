package connpool

import (
	"github.com/ecodeclub/ekit/syncx/atomicx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"testing"
)

func TestConnPool(t *testing.T) {
	webook, err := gorm.Open(mysql.Open("root:root@tcp(localhost:13316)/webook"))
	require.NoError(t, err)
	err = webook.AutoMigrate(&Interactive{})
	require.NoError(t, err)
	intr, err := gorm.Open(mysql.Open("root:root@tcp(localhost:13316)/webook_intr"))
	require.NoError(t, err)
	err = intr.AutoMigrate(&Interactive{})
	require.NoError(t, err)
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn: &DoubleWritePool{
			src:     webook.ConnPool,
			dst:     intr.ConnPool,
			pattern: atomicx.NewValueOf(PatternSrcFirst),
		},
	}))
	require.NoError(t, err)
	t.Log(db)
	err = db.Create(&Interactive{
		Biz:   "test",
		BizId: 123,
	}).Error
	require.NoError(t, err)

	err = db.Transaction(func(tx *gorm.DB) error {
		err1 := tx.Create(&Interactive{
			Biz:   "test_tx",
			BizId: 123,
		}).Error

		return err1
	})

	require.NoError(t, err)

	err = db.Model(&Interactive{}).Where("id > ?", 0).Updates(map[string]any{
		"biz_id": 789,
	}).Error
	require.NoError(t, err)
}

type Interactive struct {
	Id         int64  `gorm:"primaryKey,autoIncrement"`
	BizId      int64  `gorm:"uniqueIndex:biz_type_id"`
	Biz        string `gorm:"type:varchar(128);uniqueIndex:biz_type_id"`
	ReadCnt    int64
	CollectCnt int64
	LikeCnt    int64
	Ctime      int64
	Utime      int64
}
