package collector

import (
  "errors"
  "io/ioutil"
  "regexp"
  "sort"
  "sync"
  "time"

  "github.com/kwf2030/commons/base"
  "gopkg.in/yaml.v2"
)

var ErrDifferentRuleGroup = errors.New("different rule group")

type RuleGroup struct {
  name  string
  rules []*Rule
  mu    sync.RWMutex
}

func NewRuleGroup(name string) *RuleGroup {
  if name == "" {
    return nil
  }
  return &RuleGroup{name: name, rules: make([]*Rule, 0, 16), mu: sync.RWMutex{}}
}

func (rg *RuleGroup) match(url string) *Rule {
  if url == "" {
    return nil
  }
  rg.mu.RLock()
  defer rg.mu.RUnlock()
  for _, r := range rg.rules {
    for _, p := range r.patterns {
      if p.content.MatchString(url) {
        return r
      }
    }
  }
  return nil
}

func (rg *RuleGroup) AppendBytes(bytes []byte) error {
  if len(bytes) == 0 {
    return base.ErrInvalidArgs
  }
  r := &Rule{}
  e := yaml.Unmarshal(bytes, r)
  if e != nil {
    return e
  }
  if r.Group != rg.name {
    return ErrDifferentRuleGroup
  }
  r.init()
  rg.mu.Lock()
  defer rg.mu.Unlock()
  found := -1
  for i, old := range rg.rules {
    if old.Id == r.Id {
      found = i
      break
    }
  }
  if found == -1 {
    rg.rules = append(rg.rules, r)
  } else {
    if rg.rules[found].Version <= r.Version {
      rg.rules[found] = r
    }
  }
  sort.SliceStable(rg.rules, func(i, j int) bool {
    return rg.rules[i].Priority < rg.rules[j].Priority
  })
  return nil
}

func (rg *RuleGroup) AppendFile(file string) error {
  if file == "" {
    return base.ErrInvalidArgs
  }
  data, e := ioutil.ReadFile(file)
  if e != nil {
    return e
  }
  return rg.AppendBytes(data)
}

func (rg *RuleGroup) Remove(id string) error {
  if id == "" {
    return base.ErrInvalidArgs
  }
  rg.mu.Lock()
  defer rg.mu.Unlock()
  for i, r := range rg.rules {
    if r.Id == id {
      rg.rules = append(rg.rules[:i], rg.rules[i+1:]...)
      break
    }
  }
  return nil
}

type Rule struct {
  Id       string        `yaml:"id"`
  Version  int           `yaml:"version"`
  Name     string        `yaml:"name"`
  Alias    string        `yaml:"alias"`
  Group    string        `yaml:"group"`
  Priority int           `yaml:"priority"`
  Patterns []string      `yaml:"patterns"`
  patterns []*Pattern    `yaml:"-"`
  Prepare  *Prepare      `yaml:"prepare"`
  Timeout  string        `yaml:"timeout"`
  timeout  time.Duration `yaml:"-"`
  Fields   []*Field      `yaml:"fields"`
  Loop     *Loop         `yaml:"loop"`
}

func (r *Rule) init() {
  r.patterns = make([]*Pattern, 0, len(r.Patterns))
  for _, p := range r.Patterns {
    re, e := regexp.Compile(p)
    if e != nil {
      continue
    }
    r.patterns = append(r.patterns, &Pattern{p, re})
  }
  if r.Prepare != nil && r.Prepare.Wait != "" {
    r.Prepare.wait, _ = time.ParseDuration(r.Prepare.Wait)
  }
  r.timeout = time.Second * 10
  if r.Timeout != "" {
    r.timeout, _ = time.ParseDuration(r.Timeout)
  }
  for _, f := range r.Fields {
    if f.Wait != "" {
      f.wait, _ = time.ParseDuration(f.Wait)
    }
  }
  if r.Loop != nil {
    if r.Loop.ExportCycle == 0 {
      r.Loop.ExportCycle = 10
    }
    if r.Loop.Prepare != nil && r.Loop.Prepare.Wait != "" {
      r.Loop.Prepare.wait, _ = time.ParseDuration(r.Loop.Prepare.Wait)
    }
    if r.Loop.Wait != "" {
      r.Loop.wait, _ = time.ParseDuration(r.Loop.Wait)
    }
  }
}

type Pattern struct {
  Content string
  content *regexp.Regexp
}

type Prepare struct {
  Eval string        `yaml:"eval"`
  Wait string        `yaml:"wait"`
  wait time.Duration `yaml:"-"`
}

type Field struct {
  Name   string        `yaml:"name"`
  Alias  string        `yaml:"alias"`
  Value  string        `yaml:"value"`
  Eval   string        `yaml:"eval"`
  Export bool          `yaml:"export"`
  Wait   string        `yaml:"wait"`
  wait   time.Duration `yaml:"-"`
}

type Loop struct {
  Name        string        `yaml:"name"`
  Alias       string        `yaml:"alias"`
  ExportCycle int           `yaml:"export_cycle"`
  Prepare     *Prepare      `yaml:"prepare"`
  Eval        string        `yaml:"eval"`
  Next        string        `yaml:"next"`
  Wait        string        `yaml:"wait"`
  wait        time.Duration `yaml:"-"`
}
