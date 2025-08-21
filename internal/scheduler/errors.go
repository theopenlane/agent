package scheduler

import "errors"

var (
	// ErrCheckNotFoundInScheduler is returned when check is not found in scheduler
	ErrCheckNotFoundInScheduler = errors.New("check not found in scheduler")
	// ErrInvalidCronExpression is returned when cron expression is invalid
	ErrInvalidCronExpression = errors.New("invalid cron expression")
	// ErrInvalidInterval is returned when interval is invalid
	ErrInvalidInterval = errors.New("invalid interval")
	// ErrIntervalOutOfRange is returned when interval is out of range
	ErrIntervalOutOfRange = errors.New("interval out of range")
	// ErrInvalidNumber is returned when number is invalid
	ErrInvalidNumber = errors.New("invalid number")
	// ErrValueOutOfRange is returned when value is out of range
	ErrValueOutOfRange = errors.New("value out of range")
)
