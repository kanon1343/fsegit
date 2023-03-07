package object

type Type int

const (
	UndefinedObject Type = iota
	CommitObject
	TreeObject
	BlobObject
	TagObject
)

// オブジェクトのインデックス値を受け取り名前を返す.
func (o Type) String() string {
	objectTypeString := []string{
		"undefined",
		"commit",
		"tree",
		"blob",
		"tag",
	}
	return objectTypeString[o]
}

// 引数と合致するオブジェクトを生成
func NewType(typeString string) (objectType Type, err error) {
	switch typeString {
	case "commit":
		objectType = CommitObject
	case "tree":
		objectType = TreeObject
	case "blob":
		objectType = BlobObject
	case "tag":
		objectType = TagObject
	default:
		objectType = UndefinedObject
		err = ErrInvalidObject
	}
	return
}
