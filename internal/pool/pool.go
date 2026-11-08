// Package pool предоставляет типобезопасную обёртку над sync.Pool для объектов
// с методом Reset().
package pool

import "sync"

// Resettable — интерфейс объектов, состояние которых можно сбросить перед
// повторным использованием.
type Resettable interface {
	Reset()
}

// Pool хранит объекты одного типа и сбрасывает состояние перед возвратом в пул.
type Pool[T Resettable] struct {
	pool sync.Pool
}

// New создаёт пул. newFunc вызывается, если Get не находит объект в пуле.
func New[T Resettable](newFunc func() T) *Pool[T] {
	if newFunc == nil {
		panic("pool: nil constructor")
	}

	return &Pool[T]{
		pool: sync.Pool{
			New: func() any {
				return newFunc()
			},
		},
	}
}

// Get возвращает объект из пула или создаёт новый с помощью конструктора.
func (p *Pool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put сбрасывает состояние объекта и возвращает его в пул для повторного
// использования.
func (p *Pool[T]) Put(value T) {
	value.Reset()
	p.pool.Put(value)
}
