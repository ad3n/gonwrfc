package gorfc

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrPoolClosed        = errors.New("gorfc: connection pool is closed")
	ErrForeignConnection = errors.New("gorfc: connection does not belong to this pool")
	ErrAlreadyReleased   = errors.New("gorfc: connection was already released")
)

// PoolConfig controls the number of SAP sessions retained by a ConnectionPool.
// MaxOpen must be positive. MaxIdle is clamped to MaxOpen.
type PoolConfig struct {
	MaxOpen int
	MaxIdle int
}

// ConnectionPool is an opt-in pool of independent SAP RFC sessions. Existing
// Connection behavior remains stateful and unchanged. A checked-out connection
// must be returned with Release rather than closed directly.
type ConnectionPool struct {
	params ConnectionParameters
	idle   chan *Connection
	slots  chan struct{}
	done   chan struct{}

	managed map[*Connection]bool // true while checked out

	mu     sync.Mutex
	closed bool
}

// NewConnectionPool creates a lazy pool; it does not open a connection until
// Acquire is called. Connection parameters are copied so callers may reuse or
// modify their input map safely.
func NewConnectionPool(params ConnectionParameters, config PoolConfig) (*ConnectionPool, error) {
	if config.MaxOpen <= 0 {
		return nil, errors.New("gorfc: MaxOpen must be greater than zero")
	}
	if config.MaxIdle < 0 {
		return nil, errors.New("gorfc: MaxIdle cannot be negative")
	}
	if config.MaxIdle > config.MaxOpen {
		config.MaxIdle = config.MaxOpen
	}

	paramsCopy := make(ConnectionParameters, len(params))
	for name, value := range params {
		paramsCopy[name] = value
	}

	return &ConnectionPool{
		params:  paramsCopy,
		idle:    make(chan *Connection, config.MaxIdle),
		slots:   make(chan struct{}, config.MaxOpen),
		done:    make(chan struct{}),
		managed: make(map[*Connection]bool, config.MaxOpen),
	}, nil
}

// Acquire returns an idle connection or lazily opens a new one. It waits until
// a slot is available or ctx is cancelled.
func (pool *ConnectionPool) Acquire(ctx context.Context) (*Connection, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		// Prefer reuse before reserving capacity for a new SAP session.
		select {
		case conn := <-pool.idle:
			if pool.checkoutIdle(conn) {
				return conn, nil
			}
			continue
		default:
		}

		select {
		case <-pool.done:
			return nil, ErrPoolClosed
		case conn := <-pool.idle:
			if pool.checkoutIdle(conn) {
				return conn, nil
			}
		case pool.slots <- struct{}{}:
			conn, err := ConnectionFromParams(pool.params)
			if err != nil {
				<-pool.slots
				return nil, err
			}

			pool.mu.Lock()
			if pool.closed {
				pool.mu.Unlock()
				_ = conn.Close()
				<-pool.slots
				return nil, ErrPoolClosed
			}
			pool.managed[conn] = true
			pool.mu.Unlock()
			return conn, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (pool *ConnectionPool) checkoutIdle(conn *Connection) bool {
	pool.mu.Lock()
	_, managed := pool.managed[conn]
	if !managed || pool.closed {
		pool.mu.Unlock()
		return false
	}
	pool.managed[conn] = true
	pool.mu.Unlock()

	if conn.Alive() {
		return true
	}
	pool.discard(conn)
	return false
}

// Release returns a connection to the pool. Closed connections are discarded
// and will be replaced lazily by a later Acquire.
func (pool *ConnectionPool) Release(conn *Connection) error {
	if conn == nil {
		return ErrForeignConnection
	}

	pool.mu.Lock()
	checkedOut, managed := pool.managed[conn]
	if !managed {
		pool.mu.Unlock()
		return ErrForeignConnection
	}
	if !checkedOut {
		pool.mu.Unlock()
		return ErrAlreadyReleased
	}
	if pool.closed || !conn.Alive() {
		delete(pool.managed, conn)
		pool.mu.Unlock()
		_ = conn.Close()
		<-pool.slots
		return nil
	}

	select {
	case pool.idle <- conn:
		pool.managed[conn] = false
		pool.mu.Unlock()
		return nil
	default:
		delete(pool.managed, conn)
		pool.mu.Unlock()
		_ = conn.Close()
		<-pool.slots
		return nil
	}
}

// Close closes all currently idle connections and prevents new acquisitions.
// Connections already checked out are closed when they are released.
func (pool *ConnectionPool) Close() error {
	pool.mu.Lock()
	if pool.closed {
		pool.mu.Unlock()
		return nil
	}
	pool.closed = true
	close(pool.done)

	var firstErr error
	for {
		select {
		case conn := <-pool.idle:
			delete(pool.managed, conn)
			if err := conn.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			<-pool.slots
		default:
			pool.mu.Unlock()
			return firstErr
		}
	}
}

func (pool *ConnectionPool) discard(conn *Connection) {
	pool.mu.Lock()
	if _, ok := pool.managed[conn]; ok {
		delete(pool.managed, conn)
		pool.mu.Unlock()
		_ = conn.Close()
		<-pool.slots
		return
	}
	pool.mu.Unlock()
}
