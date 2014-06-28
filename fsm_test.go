package gofsm

import (
	"testing"
	"time"

	. "github.com/motain/gocheck"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

func (s *S) TestSimple(c *C) {
	aut, err := LoadFile("simple.dfa")
	c.Assert(err, Equals, nil)
	dog, ok := aut.Automaton["simple"]
	c.Assert(ok, Equals, true)
	c.Assert(dog.State.Name, Equals, "Hungry")

	// non-event
	dog.Process("blob")
	c.Assert(dog.State.Name, Equals, "Hungry")

	// event caught by wildcard
	dog.Process("itch.scratch")
	c.Assert(dog.State.Name, Equals, "Hungry")
	c.Assert((<-aut.Actions).String(), Equals, "scratch()")

	// event caught by wildcard
	dog.Process("sniff.nose")
	c.Assert(dog.State.Name, Equals, "Hungry")
	c.Assert((<-aut.Actions).Name, Equals, "sniff()")

	// event
	dog.Process("food.meat")
	c.Assert(dog.State.Name, Equals, "Eating")
	c.Assert((<-aut.Actions).Name, Equals, "woof()")
	c.Assert((<-aut.Actions).Name, Equals, "eat('apple')")
	ch := <-aut.Changes
	c.Assert(ch.Old, Equals, "Hungry")
	c.Assert(ch.New, Equals, "Eating")
	c.Assert(ch.Duration < time.Millisecond, Equals, true)

	dog.Process("food.meat")
	c.Assert(dog.State.Name, Equals, "Full")
	c.Assert((<-aut.Actions).Name, Equals, "groan()")
	c.Assert((<-aut.Actions).Name, Equals, "digest()")
	ch = <-aut.Changes
	c.Assert(ch.Old, Equals, "Eating")
	c.Assert(ch.New, Equals, "Full")
	c.Assert(ch.Duration < time.Millisecond, Equals, true)

	time.Sleep(time.Millisecond)

	dog.Process("run")
	c.Assert(dog.State.Name, Equals, "Hungry")
	ch = <-aut.Changes
	c.Assert(ch.Old, Equals, "Full")
	c.Assert(ch.New, Equals, "Hungry")
	c.Assert(ch.Duration > time.Millisecond, Equals, true)

	c.Assert(aut.String(), Equals, "simple: Hungry")
}

func (s *S) TestPersistRestore(c *C) {
	aut, err := LoadFile("simple.dfa")
	c.Assert(err, Equals, nil)
	dog, _ := aut.Automaton["simple"]
	c.Assert(dog.State.Name, Equals, "Hungry")

	p := aut.Persist()

	aut, err = LoadFile("simple.dfa")
	aut.Restore(p)
	dog, _ = aut.Automaton["simple"]
	c.Assert(dog.State.Name, Equals, "Hungry")
}

func (s *S) TestInvalid(c *C) {
	conf := "invalid: {}"
	_, err := Load([]byte(conf))
	c.Assert(err, NotNil)
}
