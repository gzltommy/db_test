package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"time"
)

var (
	redisPool       *redis.Pool
	redisNotInitErr = errors.New("redis is not initialized.")
	redisUnlockErr  = errors.New("only the thread holding the lock can unlock")
	KeyNotExist     = errors.New("key does not exist")
)

func init() {
	redisHost := "192.168.24.133"
	redisPort := "6379"
	password := ""

	url := fmt.Sprintf("%s:%s", redisHost, redisPort)
	redisPool = &redis.Pool{
		TestOnBorrow:    nil,
		MaxIdle:         20,
		MaxActive:       100,
		IdleTimeout:     20 * time.Second,
		Wait:            false,
		MaxConnLifetime: 0,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", url)
			if err != nil {
				return nil, err
			}
			_, err = c.Do("select", 1)
			if err != nil {
				return nil, err
			}
			if password != "" {
				_, err = c.Do("AUTH", password)
				if err != nil {
					return nil, err
				}
			}
			return c, nil
		},
	}

	return
}

func Close() error {
	if redisPool == nil {
		return nil
	}
	if err := redisPool.Close(); err != nil {
		fmt.Printf("redis close err:%v", err)
	} else {
		fmt.Println("redis close")
	}
	return nil
}

func Get(key string) ([]byte, error) {
	if redisPool == nil {
		return nil, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()

	reply, err := c.Do("GET", key)
	if err != nil {
		return nil, err
	}
	val, ok := reply.([]byte)
	if ok {
		return val, nil
	}
	return nil, KeyNotExist
}

func MGet(keys ...interface{}) ([]interface{}, error) {
	if redisPool == nil {
		return nil, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()

	return redis.Values(c.Do("mget", keys...))
}

func Set(key string, ttl int64, value []byte) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	if _, err := c.Do("set", key, value); err != nil {
		return err
	}
	if ttl > 0 {
		if _, err := c.Do("expire", key, ttl); err != nil {
			return err
		}
	}
	return nil
}

func SetInt(key string, ttl int64, value int64) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	if _, err := c.Do("set", key, value); err != nil {
		return err
	}
	if ttl > 0 {
		if _, err := c.Do("expire", key, ttl); err != nil {
			return err
		}
	}
	return nil
}

func GetInt(key string) (int64, error) {
	if redisPool == nil {
		return 0, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Int64(c.Do("GET", key))
}

func DecrBy(key string, value interface{}) (int64, error) {
	if redisPool == nil {
		return 0, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Int64(c.Do("DECRBY", key, value))
}

func IncrBy(key string, value interface{}) (int64, error) {
	if redisPool == nil {
		return 0, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Int64(c.Do("INCRBY", key, value))
}

func CoinIdIncrBy(key string, value interface{}) (int64, error) {
	if redisPool == nil {
		return 0, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Int64(c.Do("INCRBY", key, value))
}

func SetIfNotExistUnsafe(key string, ttl int64, value int64) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	_, err := redis.Int64(c.Do("GET", key))
	if err == redis.ErrNil {
		if _, err := c.Do("set", key, value); err != nil {
			return err
		}
		if ttl > 0 {
			if _, err := c.Do("expire", key, ttl); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

func Del(key string) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	_, err := c.Do("del", key)
	return err
}

func RPush(key string, ttl int64, args ...[]byte) (int64, error) {
	if redisPool == nil {
		panic(nil)
	}
	c := redisPool.Get()
	defer c.Close()
	vs := []interface{}{}
	vs = append(vs, key)
	for _, v := range args {
		vs = append(vs, v)
	}
	reply, err := redis.Int64(c.Do("RPUSH", vs...))
	if err != nil {
		return 0, err
	}

	if ttl > 0 {
		if _, err := c.Do("expire", key, ttl); err != nil {
			return 0, err
		}
	}

	return reply, nil
}

func LPush(key string, ttl int64, args ...[]byte) (int64, error) {
	if redisPool == nil {
		panic(nil)
	}
	c := redisPool.Get()
	defer c.Close()
	vs := []interface{}{}
	vs = append(vs, key)
	for _, v := range args {
		vs = append(vs, v)
	}
	reply, err := redis.Int64(c.Do("LPUSH", vs...))
	if ttl > 0 {
		if _, err := c.Do("expire", key, ttl); err != nil {
			return 0, err
		}
	}
	return reply, err
}

type processFunc func(string, []byte)
type subscribeFunc func([]byte) bool

func ListenRedisList(ctx context.Context, queueName string, fn processFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			calList(queueName, fn)
		}
	}
	return
}

func calList(queueName string, fn processFunc) {
	c := redisPool.Get()
	defer c.Close()
	reply, err := redis.ByteSlices(c.Do("BRPOP", queueName, 10))
	if err != nil {
		if err != redis.ErrNil {

		}
		return
	}
	if len(reply) == 2 {
		fn(queueName, reply[1])
	}
}

func PubRedisList(queueName string, payload interface{}) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = LPush(queueName, 0, bytes)
	if err != nil {
		return err
	}
	return nil
}

func PublishChannel(channel string, message string) (int64, error) {
	c := redisPool.Get()
	defer c.Close()
	reply, err := redis.Int64(c.Do("PUBLISH", channel, message))
	return reply, err
}

func PSubscribe(channel string, callback subscribeFunc) error {
	c := redisPool.Get()
	defer c.Close()
	psc := redis.PubSubConn{Conn: c}
	psc.Subscribe(channel)
	for {
		switch v := psc.Receive().(type) {
		case error:
			psc.Unsubscribe(channel)
			return v
		case redis.Message:
			{
				if !callback(v.Data) {
					psc.Unsubscribe(channel)
					return nil
				}
				continue
			}
		}
	}
	return nil
}

func HmSet(key string, ttl int64, kvPairs ...interface{}) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	if _, err := c.Do("hmset", append([]interface{}{key}, kvPairs...)...); err != nil {
		return err
	}
	if ttl > 0 {
		if _, err := c.Do("expire", key, ttl); err != nil {
			return err
		}
	}
	return nil
}

func HmGet(key string, ks ...interface{}) ([]interface{}, error) {
	if redisPool == nil {
		return nil, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Values(c.Do("hmget", append([]interface{}{key}, ks...)...))
}

func HmDel(key string, ks ...interface{}) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	_, err := c.Do("hdel", append([]interface{}{key}, ks...)...)
	return err
}

// smPairs：score1 member1 [score2 member2]
func ZAdd(key string, ttl int64, smPairs ...interface{}) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	if _, err := c.Do("zadd", append([]interface{}{key}, smPairs...)...); err != nil {
		return err
	}
	if ttl > 0 {
		if _, err := c.Do("expire", key, ttl); err != nil {
			return err
		}
	}
	return nil
}

func LIndex(key string, index int) (interface{}, error) {
	if redisPool == nil {
		return nil, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return c.Do("LINDEX", key, index)
}

func ZRange(key string, start, stop int) ([]interface{}, error) {
	if redisPool == nil {
		return nil, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Values(c.Do("zrange", key, start, stop))
}

func ZRem(key string, ms ...interface{}) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	_, err := c.Do("zrem", append([]interface{}{key}, ms...)...)
	return err
}

func LRange(key string, start, stop int) ([]interface{}, error) {
	if redisPool == nil {
		return nil, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Values(c.Do("LRANGE", key, start, stop))
}

func LLen(key string) (int64, error) {
	if redisPool == nil {
		return 0, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Int64(c.Do("LLEN", key))
}

func TryLock(lockName string, ttl int64) (error, bool) {
	if redisPool == nil {
		return redisNotInitErr, false
	}
	c := redisPool.Get()
	defer c.Close()
	result, err := redis.String(c.Do("SET", lockName, []byte{1}, "EX", ttl, "NX"))
	if err == nil && result == "OK" {
		return nil, true
	}
	return nil, false
}

func Unlock(lockName string) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	_, err := c.Do("DEL", lockName)
	return err
}

func ZrevRange(key string, start, end int) ([]int64, error) {
	if redisPool == nil {
		return []int64{}, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Int64s(c.Do("ZREVRANGE", key, start, end))
}

func ZrevRangeWithScore(key string, start, end int) ([]int64, error) {
	if redisPool == nil {
		return nil, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()

	return redis.Int64s(c.Do("ZREVRANGE", key, start, end, "withscores"))
}
func Zrem(key string, member interface{}) error {
	if redisPool == nil {
		return redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	_, err := c.Do("ZREM", key, member)
	return err
}
func ZCard(key string) (int64, error) {
	if redisPool == nil {
		return 0, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Int64(c.Do("zcard", key))
}

func ZLEXCOUNT(key string, min, max interface{}) (int64, error) {
	if redisPool == nil {
		return 0, redisNotInitErr
	}
	c := redisPool.Get()
	defer c.Close()
	return redis.Int64(c.Do("ZLEXCOUNT", key, min, max))
}
