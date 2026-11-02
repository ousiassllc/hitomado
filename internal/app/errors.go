package app

import "errors"

// errNotImplemented は実装前のサービスメソッド用（実装が揃った時点で削除する）。
var errNotImplemented = errors.New("not implemented")
