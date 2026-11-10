package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/worker4556/naparnik/backend/internal/store"
)

var rejected = errors.New("action rejected")

// atomic — проверка и изменение составляют одну операцию, а не два независимых
// обращения к store. Ошибка действия или записи отменяет все изменения.
func atomic[T any](c Ctx, fn func(Ctx) (T, *Error)) (T, *Error) {
	var value T
	var actionErr *Error
	err := c.Store.Transaction(func(s *store.Store) error {
		value, actionErr = fn(Ctx{Store: s, User: c.User})
		if actionErr != nil {
			return rejected
		}
		return nil
	})
	if err != nil && actionErr == nil {
		actionErr = &Error{Code: "unavailable", Message: "Не удалось сохранить действие. Повтори запрос."}
	}
	if actionErr != nil {
		var zero T
		return zero, actionErr
	}
	return value, nil
}

func atomicResult(c Ctx, fn func(Ctx) Result) Result {
	res, e := atomic(c, func(c Ctx) (Result, *Error) { r := fn(c); return r, r.Error })
	if e != nil {
		return Result{Error: e}
	}
	return res
}

// CreateSelfOnce — ключ относится к пользователю и операции. Повтор с тем же
// содержимым возвращает первую квитанцию, другое содержимое с тем же ключом запрещено.
func CreateSelfOnce(c Ctx, key, kind string, args map[string]any) Result {
	if key == "" || len(key) > 128 {
		return fail("validation", "Нужен Idempotency-Key длиной до 128 символов.")
	}
	raw, err := json.Marshal(struct {
		Kind string
		Args map[string]any
	}{kind, args})
	if err != nil {
		return fail("validation", "Некорректная заявка.")
	}
	hash := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(hash[:])
	scope := c.User.ID + ":create-request:" + key
	return atomicResult(c, func(c Ctx) Result {
		if previous, ok := c.Store.Operation(scope); ok {
			if previous.Fingerprint != fingerprint {
				return fail("validation", "Этот Idempotency-Key уже использован для другой заявки.")
			}
			var result Result
			if json.Unmarshal(previous.Result, &result) != nil {
				return fail("unavailable", "Не удалось прочитать результат заявки.")
			}
			return result
		}
		res := CreateSelf(c, kind, args)
		if res.OK {
			raw, _ := json.Marshal(res)
			c.Store.PutOperation(scope, store.Operation{Fingerprint: fingerprint, Result: raw})
		}
		return res
	})
}
