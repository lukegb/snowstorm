/*
Copyright 2017 Luke Granger-Brown

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import "io"

type wrappedCloser struct {
	r io.Reader
	c io.Closer
}

func (wc *wrappedCloser) Read(b []byte) (n int, err error) {
	if wc.r == nil {
		return 0, io.ErrClosedPipe
	}
	return wc.r.Read(b)
}

func (wc *wrappedCloser) Close() error {
	if wc.c == nil {
		return nil
	}

	err := wc.c.Close()
	if err != nil {
		return err
	}

	wc.c = nil
	wc.r = nil
	return nil
}

func newWrappedCloser(r io.Reader, c io.Closer) io.ReadCloser {
	return &wrappedCloser{r, c}
}
