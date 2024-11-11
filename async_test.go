package async_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maratori/async"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutor(t *testing.T) {
	t.Parallel()
	t.Run("executor is error-prone (is it possible to fix?)", func(t *testing.T) {
		t.Parallel()
		t.Run("using iota is dangerous", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			executor := async.NewExecutor()
			_, err := executor.Register("job", domainService, DomainService.Struct__MyEnum__Nothing)
			require.NoError(t, err)
			// Under the hood [MyEnumB] is converted to `2` and is stored for the future execution.
			// If someone adds/removes enum const before [MyEnumB], it changes numbering (actual values of enums).
			// It means that once job enqueued with argument `2` (meaning [MyEnumB]) will be executed with argument 2.
			// But now it means not [MyEnumB], but some different enum const.
			_, args, err := executor.Prepare(DomainService.Struct__MyEnum__Nothing, MyEnumB)
			require.NoError(t, err)
			assert.JSONEq(t, `{"arg0":2,"type0":"int"}`, string(args))
		})
	})
	t.Run("positive", func(t *testing.T) {
		t.Parallel()
		t.Run("register and use two method of same type", func(t *testing.T) {
			t.Parallel()
			// These two methods have the same type (signature),
			// but we get method name from runtime to distinguish between them
			require.Equal(t,
				reflect.TypeOf(DomainService.Struct__String__Nothing),
				reflect.TypeOf(DomainService.OneMoreMethodSameSignature_Struct__String__Nothing),
			)
			const arg = "xyz"
			ctx := context.Background()
			executor := async.NewExecutor()
			domainService := NewDomainService()
			handler1, err := executor.Register("job1", domainService, DomainService.Struct__String__Nothing)
			require.NoError(t, err)
			handler2, err := executor.Register("job2", domainService, DomainService.OneMoreMethodSameSignature_Struct__String__Nothing) //nolint:lll
			require.NoError(t, err)
			name1, data1, err := executor.Prepare(DomainService.Struct__String__Nothing, arg)
			require.NoError(t, err)
			name2, data2, err := executor.Prepare(DomainService.OneMoreMethodSameSignature_Struct__String__Nothing, arg)
			require.NoError(t, err)
			assert.Equal(t, "job1", name1)
			assert.Equal(t, "job2", name2)
			assert.Equal(t, data1, data2)
			assert.NotPanics(t, func() {
				require.NoError(t, handler1(ctx, data1))
			})
			assert.PanicsWithValue(t, arg, func() {
				_ = handler2(ctx, data2)
			})
			requireSingleElemInChan(t, arg, domainService.ch)
		})
		registerAndCall := func(t *testing.T, receiver interface{}, method interface{}, args ...interface{}) error {
			ctx := context.Background()
			executor := async.NewExecutor()
			handler, err := executor.Register("job name", receiver, method)
			require.NoError(t, err)
			name, data, err := executor.Prepare(method, args...)
			require.NoError(t, err)
			require.Equal(t, "job name", name)
			return handler(ctx, data)
		}
		t.Run("Struct__Nothing__Nothing", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__Nothing__Nothing)
			require.NoError(t, err)
			requireSingleElemInChan(t, executed, domainService.ch)
		})
		t.Run("Struct__String__Nothing", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__String__Nothing, "abc")
			require.NoError(t, err)
			requireSingleElemInChan(t, "abc", domainService.ch)
		})
		t.Run("Ptr__String__Nothing", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, &domainService, (*DomainService).Ptr__String__Nothing, "abc")
			require.NoError(t, err)
			requireSingleElemInChan(t, "abc", domainService.ch)
		})
		t.Run("Struct__MyString__Nothing", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__MyString__Nothing, MyString("abc"))
			require.NoError(t, err)
			requireSingleElemInChan(t, "abc", domainService.ch)
		})
		t.Run("Struct__Context_String__Nothing", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__Context_String__Nothing, "abc")
			require.NoError(t, err)
			requireSingleElemInChan(t, "abc", domainService.ch)
		})
		t.Run("Struct__Context_String__Error - no error", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__Context_String__Error, "abc")
			require.NoError(t, err)
			requireSingleElemInChan(t, "abc", domainService.ch)
		})
		t.Run("Struct__Context_String__Error - error", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__Context_String__Error, errorPrefix+"abc")
			require.EqualError(t, err, "abc")
			requireChanIsEmpty(t, domainService.ch)
		})
		t.Run("Struct__Context_String__MyError - no error", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__Context_String__MyError, "abc")
			require.NoError(t, err)
			requireSingleElemInChan(t, "abc", domainService.ch)
		})
		t.Run("Struct__Context_String__MyError - error", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__Context_String__MyError, errorPrefix+"abc")
			require.EqualError(t, err, "abc")
			assert.IsType(t, new(MyError), err)
			requireChanIsEmpty(t, domainService.ch)
		})
		t.Run("Struct__MyEmptyStruct_String__Nothing", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			err := registerAndCall(t, domainService, DomainService.Struct__MyEmptyStruct_String__Nothing, MyEmptyStruct{}, "abc")
			require.NoError(t, err)
			requireSingleElemInChan(t, "abc", domainService.ch)
		})
	})
	t.Run("negative", func(t *testing.T) {
		t.Parallel()
		t.Run("nil arg (may be allowed in the future)", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			executor := async.NewExecutor()
			_, err := executor.Register("job", domainService, DomainService.Struct__String__Nothing)
			require.NoError(t, err)
			_, _, err = executor.Prepare(DomainService.Struct__String__Nothing, nil)
			require.EqualError(t, err, "args[0]: expected string, received %!s(<nil>)")
			requireChanIsEmpty(t, domainService.ch)
		})
		t.Run("string instead of MyString (may be allowed in the future)", func(t *testing.T) {
			t.Parallel()
			domainService := NewDomainService()
			executor := async.NewExecutor()
			_, err := executor.Register("job", domainService, DomainService.Struct__MyString__Nothing)
			require.NoError(t, err)
			_, _, err = executor.Prepare(DomainService.Struct__MyString__Nothing, "abc")
			require.EqualError(t, err, "args[0]: expected async_test.MyString, received string")
			requireChanIsEmpty(t, domainService.ch)
		})
	})
}

type DomainService struct { //nolint:recvcheck // intentional mix receiver types
	ch chan string
}

func NewDomainService() DomainService {
	return DomainService{
		ch: make(chan string, 10),
	}
}

func (d DomainService) Struct__Nothing__Nothing() {
	d.ch <- executed
}

func (d DomainService) Struct__String__Nothing(s string) {
	d.ch <- s
}

func (d DomainService) OneMoreMethodSameSignature_Struct__String__Nothing(s string) {
	panic(s)
}

func (d *DomainService) Ptr__String__Nothing(s string) {
	d.ch <- s
}

func (d DomainService) Struct__MyString__Nothing(s MyString) {
	d.ch <- string(s)
}

func (d DomainService) Struct__Context_String__Nothing(_ context.Context, s string) {
	d.ch <- s
}

func (d DomainService) Struct__Context_String__Error(_ context.Context, s string) error {
	if strings.HasPrefix(s, errorPrefix) {
		return errors.New(strings.TrimPrefix(s, errorPrefix))
	}
	d.ch <- s
	return nil
}

func (d DomainService) Struct__Context_String__MyError(_ context.Context, s string) *MyError {
	if strings.HasPrefix(s, errorPrefix) {
		e := strings.TrimPrefix(s, errorPrefix)
		return (*MyError)(&e)
	}
	d.ch <- s
	return nil
}

func (d DomainService) Struct__MyEnum__Nothing(e MyEnum) {
	d.ch <- strconv.Itoa(int(e))
}

func (d DomainService) Struct__MyEmptyStruct_String__Nothing(_ MyEmptyStruct, s string) {
	d.ch <- s
}

const executed = "executed"
const errorPrefix = "error prefix: "

type MyString string

type MyEnum int

const (
	MyEnumA MyEnum = iota + 1
	MyEnumB
	MyEnumC
)

type MyEmptyStruct struct{}

type MyError string

func (e *MyError) Error() string {
	if e == nil {
		return ""
	}
	return string(*e)
}

func requireSingleElemInChan(t *testing.T, expected string, ch <-chan string) {
	select {
	case <-time.After(1 * time.Second):
		require.Fail(t, "timeout")
	case actual := <-ch:
		require.Equal(t, expected, actual)
	}
	requireChanIsEmpty(t, ch)
}

func requireChanIsEmpty(t *testing.T, ch <-chan string) {
	select {
	case <-time.After(1 * time.Second):
	case elem := <-ch:
		require.Failf(t, "String channel is not empty", "%q received", elem)
	}
}
