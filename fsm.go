package gofsm

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/v1/yaml"
)

type Automata struct {
	Automaton map[string]*Automaton
	Actions   chan Action
	Changes   chan Change
}

type State struct {
	Name     string
	Steps    []Step
	Entering Actions
	Leaving  Actions
}

// Condition is an expression of the form:
// condition1 or condition2 or ...
// Each condition can use wildcards, eg 'pir.*'
type Condition struct {
	When string
}

type Step struct {
	When    Condition
	Actions Actions
	Next    string
}

type Actions []string

type Transition struct {
	When    string
	Actions Actions
}

type Automaton struct {
	Start  string
	States map[string]struct {
		Entering Actions
		Leaving  Actions
	}
	Transitions map[string][]Transition
	Name        string
	State       *State
	Since       time.Time
	actions     chan Action
	changes     chan Change
	sm          map[string]*State
}

type Action struct {
	Name    string
	Trigger interface{}
}

type Change struct {
	Automaton string
	Old       string
	New       string
	Since     time.Time
	Duration  time.Duration
}

func (self Action) String() string {
	return self.Name
}

func (self Condition) Match(s string) bool {
	conds := strings.Split(self.When, " or ")
	matched := false
	for _, cond := range conds {
		// borrow filepath globbing
		matched, _ = filepath.Match(cond, s)
		if matched {
			break
		}
	}
	return matched
}

func (self *Automata) Process(event interface{}) {
	for _, aut := range self.Automaton {
		aut.Process(event)
	}
}

func (self *Automata) String() string {
	var out string
	for k, aut := range self.Automaton {
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf("%s: %s", k, aut.State.Name)
	}
	return out
}

func (self *Automaton) Process(event interface{}) {
	str := fmt.Sprint(event)
	for _, t := range self.State.Steps {
		if t.When.Match(str) {
			// emit leaving actions
			for _, action := range self.State.Leaving {
				self.actions <- Action{action, event}
			}
			// emit transition actions
			for _, action := range t.Actions {
				self.actions <- Action{action, event}
			}
			// change state
			if self.State.Name != t.Next {
				old := self.State.Name
				oldSince := self.Since
				self.State = self.sm[t.Next]
				self.Since = time.Now()
				duration := self.Since.Sub(oldSince)
				self.changes <- Change{Automaton: self.Name, Old: old, New: t.Next, Duration: duration, Since: oldSince}
			}
			// emit entering actions
			for _, action := range self.State.Entering {
				self.actions <- Action{action, event}
			}
		}
	}
}

func (self *Automaton) load() error {
	if self.Start == "" {
		return errors.New("missing Start entry")
	}
	if len(self.States) == 0 {
		return errors.New("missing States entries")
	}
	if len(self.Transitions) == 0 {
		return errors.New("missing Transitions entries")
	}

	sm := map[string]*State{}

	var allStates []string
	for name, val := range self.States {
		state := State{Name: name}
		state.Entering = val.Entering
		state.Leaving = val.Leaving
		sm[name] = &state

		allStates = append(allStates, name)
	}
	self.sm = sm

	var ok bool
	if self.State, ok = sm[self.Start]; !ok {
		return errors.New("starting State invalid")
	}
	self.Since = time.Now()

	type StringPair struct {
		_1 string
		_2 string
	}

	for name, trans := range self.Transitions {
		var pairs []StringPair
		lr := strings.SplitN(name, "->", 2)
		if len(lr) == 2 {
			// from->to
			var froms, tos []string
			if lr[0] == "*" {
				froms = allStates
			} else {
				froms = strings.Split(lr[0], ",")
			}
			if lr[1] == "*" {
				tos = allStates
			} else {
				tos = strings.Split(lr[1], ",")
			}
			for _, f := range froms {
				for _, t := range tos {
					pairs = append(pairs, StringPair{f, t})
				}
			}
		} else {
			// from1,from2 = from1->from1, from2->from2
			var froms []string
			if lr[0] == "*" {
				froms = allStates
			} else {
				froms = strings.Split(lr[0], ",")
			}
			for _, f := range froms {
				pairs = append(pairs, StringPair{f, f})
			}
		}

		for _, pair := range pairs {
			from, to := pair._1, pair._2
			var sfrom *State
			if sfrom, ok = self.sm[from]; !ok {
				return errors.New(fmt.Sprintf("State: %s not found", from))
			}
			if _, ok := self.sm[to]; !ok {
				return errors.New(fmt.Sprintf("State: %s not found", from))
			}

			for _, v := range trans {
				t := Step{Condition{v.When}, v.Actions, to}
				sfrom.Steps = append(sfrom.Steps, t)
			}
		}
	}
	return nil
}

type AutomataState map[string]AutomatonState

type AutomatonState struct {
	State string
	Since time.Time
}

func (self *Automata) Persist() AutomataState {
	ret := AutomataState{}
	for k, aut := range self.Automaton {
		ret[k] = AutomatonState{aut.State.Name, aut.Since}
	}
	return ret
}

func (self *Automata) Restore(s AutomataState) {
	for k, as := range s {
		if aut, ok := self.Automaton[k]; ok {
			aut.State, _ = aut.sm[as.State]
			aut.Since = as.Since
		}
	}
}

func LoadFile(filename string) (*Automata, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return Load(data)
}

func Load(str []byte) (*Automata, error) {
	var aut Automata = Automata{Actions: make(chan Action, 32), Changes: make(chan Change, 32)}
	err := yaml.Unmarshal(str, &aut.Automaton)
	for k, a := range aut.Automaton {
		err := a.load()
		if err != nil {
			return nil, errors.New(fmt.Sprintf("%s: %s", k, err.Error()))
		}
		a.Name = k
		a.actions = aut.Actions
		a.changes = aut.Changes
	}

	return &aut, err
}
