package rbac

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/klog/v2"
)

//go:embed user_perm_table.json
var content embed.FS

// method 1: where clause
// method 2: user_perm_table table
// method 3: Materialized view - creation at attempt 1 and then cache MV names for later attempts
//insert into db rbacTimes userUID, Method, Function, timetaken, created
var Users []string
var Options []string

// var UserPerm []map[string]interface{} // for stack of records
var UserPerm map[string]interface{} // for stack of records
var UserMV map[string]string        // for stack of records

type RbacRecord struct {
	Pool                      pgxpoolmock.PgxPool
	UserUID, Option, Function string
	TimeTaken                 time.Duration
	Created                   time.Time
	Result                    int
	MVPresent                 bool
}

func init() {
	Users = []string{"user2", "user3", "user1"}
	Options = []string{"whereClause", "Table", "matView"}
	UserMV = make(map[string]string, len(Users))
	_, err := db.GetConnection().Query(context.TODO(), "DROP TABLE search.rbacQueryTimes")
	fmt.Println("Err dropping rbacQueryTimes table", err)

	_, err = db.GetConnection().Query(context.TODO(), "CREATE TABLE IF NOT EXISTS search.rbacQueryTimes (uid TEXT, option TEXT,function TEXT, timeTaken TEXT,created timestamp, result integer, mvPresent BOOLEAN)")
	if err != nil {
		fmt.Println("Err creating rbacQueryTimes table", err)
	}
	fmt.Println("Created rbacQueryTimes table")
	loadUserPerm()
}

func InsertRbacTimes(s RbacRecord) {
	sql := "INSERT into search.rbacQueryTimes values($1,$2,$3,$4,$5,$6,$7)"
	args := []interface{}{s.UserUID, s.Option, s.Function, s.TimeTaken.String(), s.Created, s.Result, s.MVPresent}
	_, err := s.Pool.Query(context.TODO(), sql, args...)
	if err != nil {
		fmt.Println("Err Inserting result into Rbac times table: ", err)
	} else {
		fmt.Println("Inserted result into Rbac times table")
	}
}

func loadUserPerm() {
	// Read json file and build mock data
	if bytes, err := content.ReadFile("user_perm_table.json"); err == nil {
		fmt.Println("Read file without err:", err)

		// var v interface{}

		if err := json.Unmarshal(bytes, &UserPerm); err != nil {
			fmt.Println("Err reading file and loading user_perm: ", err)
			panic(err)
		}
		fmt.Println("Read file and loaded user_perm.")
		// fmt.Printf("UserPerm:%+v\n", UserPerm)
		fmt.Println("type: ", reflect.TypeOf(UserPerm))
		// fmt.Println("value: ", reflect.ValueOf(UserPerm))
		fmt.Println("1: ", UserPerm["user1"])
		fmt.Println("2: ", UserPerm["user2"])
		fmt.Println("3: ", UserPerm["user3"])

	} else {
		fmt.Println("Read file err:", err)
		panic(err)
	}

}

func CheckTable(user string) (string, bool) {
	if mv, ok := UserMV[user]; ok {
		return mv, true
	}
	return "", false
}

func GetUserPermissions(user string) exp.ExpressionList {
	whereOr := make(map[int]exp.ExpressionList, 1)

	if perm, ok := UserPerm[user]; ok {
		mapArray, isMapArray := perm.([]interface{})

		fmt.Println("len whereOr:", len(whereOr))
		if isMapArray {
			for i, ma := range mapArray {
				ma, _ := ma.(map[string]interface{})
				// fmt.Printf("user2 maok: %t\n", maok)
				// fmt.Printf("user2 ma: +%v\n", ma)
				var whereOrDs []exp.Expression

				whereOrDs = append(whereOrDs, goqu.COALESCE(goqu.L(`data->>?`, "apigroup"), "").In(ma["apigroup"]),
					goqu.L(`data->>?`, "kind").In(ma["kind"]),
					goqu.L(`data->>?`, "namespace").In(ma["namespace"]))
				// fmt.Println("i:", i)
				if i == 0 {
					whereOr[0] = goqu.And(whereOrDs...)
				} else {
					whereOr[0] = goqu.Or(whereOr[0], goqu.And(whereOrDs...))
				}
				// fmt.Println("whereOr after insertion:", whereOr)
				// whereDs = append(whereDs, whereOrDs...)
				// fmt.Println("whereOr inside", whereOr, " for user", user)

			}
			// return whereOr[0]
		} else {
			fmt.Println("isMapArray?", isMapArray)
			// fmt.Println("type perm?", reflect.TypeOf(perm))
			// fmt.Println(" perm?", perm)
		}
	} else {
		klog.Error("No permission data exists for user ", user)
	}
	// fmt.Println("whereOr outside", whereOr, " for user", user)
	return whereOr[0]
}
