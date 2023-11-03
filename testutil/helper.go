// Copyright 2023 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"strconv"
	"time"
)

type Config struct {
	Account   bool
	Pets      int
	Toys      int
	Company   bool
	Manager   bool
	Team      int
	Languages int
	Friends   int
	NamedPet  bool
}

func GetUser(name string, config Config) *User {
	var (
		birthday = time.Now().Round(time.Second)
		user     = User{
			Name:     name,
			Age:      18,
			Birthday: &birthday,
		}
	)

	if config.Account {
		user.Account = Account{Number: name + "_account"}
	}

	for i := 0; i < config.Pets; i++ {
		user.Pets = append(user.Pets, &Pet{Name: name + "_pet_" + strconv.Itoa(i+1)})
	}

	for i := 0; i < config.Toys; i++ {
		user.Toys = append(user.Toys, Toy{Name: name + "_toy_" + strconv.Itoa(i+1)})
	}

	if config.Company {
		user.Company = Company{Name: "company-" + name}
	}

	if config.Manager {
		user.Manager = GetUser(name+"_manager", Config{})
	}

	for i := 0; i < config.Team; i++ {
		user.Team = append(user.Team, *GetUser(name+"_team_"+strconv.Itoa(i+1), Config{}))
	}

	for i := 0; i < config.Languages; i++ {
		name := name + "_locale_" + strconv.Itoa(i+1)
		language := Language{Code: name, Name: name}
		user.Languages = append(user.Languages, language)
	}

	for i := 0; i < config.Friends; i++ {
		user.Friends = append(user.Friends, GetUser(name+"_friend_"+strconv.Itoa(i+1), Config{}))
	}

	if config.NamedPet {
		user.NamedPet = &Pet{Name: name + "_namepet"}
	}

	return &user
}
