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

package mndx

import (
	"context"

	"github.com/lukegb/snowstorm/ngdp/client"
	"github.com/pkg/errors"
)

// Decorate adds a FilenameMapper to the provided client.
//
// It will automatically download and parse the root file.
func Decorate(ctx context.Context, c *client.Client) error {
	root, err := c.Fetch(ctx, c.BuildConfig.Root)
	if err != nil {
		return errors.Wrap(err, "fetching root file")
	}
	defer root.Body.Close()

	mapper, err := Parse(root.Body)
	if err != nil {
		return errors.Wrap(err, "parsing root file")
	}

	tree, err := ToTree(mapper)
	if err != nil {
		return errors.Wrap(err, "converting to tree")
	}

	c.FilenameMapper = tree
	return nil
}
