package lru_k

type Option func(*cache)

func WithK(k int) Option {
	return func(c *cache) {
		c.k = k
	}
}

func WithOnEliminate(onEliminate func(k string, v any)) Option {
	return func(c *cache) {
		c.onEliminate = onEliminate
	}
}
