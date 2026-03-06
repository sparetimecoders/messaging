// MIT License
//
// Copyright (c) 2026 sparetimecoders
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package messaging

import (
	"context"
	"errors"
)

// ErrParseJSON is returned when the event payload cannot be parsed as JSON.
var ErrParseJSON = errors.New("failed to parse")

// EventHandler is a function that is used to handle events of a specific type.
// If processing fails, an error should be returned and the message will be re-queued.
type EventHandler[T any] func(ctx context.Context, event ConsumableEvent[T]) error

// RequestResponseEventHandler is a function that is used to handle events of a specific
// type and return a response with RequestResponseHandler.
// If processing fails, an error should be returned and the message will be re-queued.
type RequestResponseEventHandler[T any, R any] func(ctx context.Context, event ConsumableEvent[T]) (R, error)
