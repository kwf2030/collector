package crawler

import (
  "errors"
  "io/ioutil"
  "regexp"
  "sort"
  "sync"
  "time"

  "gopkg.in/yaml.v2"
)

var (
  ErrGroupNotFound = errors.New("group not found")

  Rules = &RuleGroups{Groups: make(map[string][]*Rule, 16), RWMutex: &sync.RWMutex{}}
)

type RuleGroups struct {
  Groups map[string][]*Rule
  *sync.RWMutex
}

func (rg *RuleGroups) match(group, url string) *Rule {
  if group == "" || url == "" {
    return nil
  }
  rg.RLock()
  defer rg.RUnlock()
  arr, ok := rg.Groups[group]
  if !ok {
    return nil
  }
  for _, r := range arr {
    for _, p := range r.patterns {
      if p.content.MatchString(url) {
        return r
      }
    }
  }
  return nil
}

func (rg *RuleGroups) FromBytes(bytes [][]byte) error {
  if len(bytes) == 0 {
    return ErrInvalidArgs
  }
  m := make(map[string][]*Rule, len(bytes))
  for _, b := range bytes {
    r := &Rule{}
    e := yaml.Unmarshal(b, r)
    if e != nil {
      return e
    }
    initRule(r)
    if _, ok := m[r.Group]; !ok {
      m[r.Group] = make([]*Rule, 0, 16)
    }
    m[r.Group] = append(m[r.Group], r)
  }
  for g, r := range m {
    e := rg.update(g, r)
    if e != nil {
      return e
    }
  }
  return nil
}

func (rg *RuleGroups) FromFiles(files []string) error {
  if len(files) == 0 {
    return ErrInvalidArgs
  }
  arr := make([][]byte, 0, len(files))
  for _, f := range files {
    data, e := ioutil.ReadFile(f)
    if e != nil {
      return e
    }
    arr = append(arr, data)
  }
  return rg.FromBytes(arr)
}

func (rg *RuleGroups) update(group string, arr []*Rule) error {
  if group == "" || len(arr) == 0 {
    return ErrInvalidArgs
  }
  rg.Lock()
  defer rg.Unlock()
  if _, ok := rg.Groups[group]; !ok {
    rg.Groups[group] = make([]*Rule, 0, 16)
  }
  for _, r := range arr {
    if r.Group != group {
      continue
    }
    index := -1
    for i, old := range rg.Groups[group] {
      if old.Id == r.Id {
        index = i
        break
      }
    }
    if index == -1 {
      rg.Groups[group] = append(rg.Groups[group], r)
    } else {
      old := rg.Groups[group][index]
      if old.Version < r.Version {
        rg.Groups[group][index] = r
      }
    }
  }
  sort.SliceStable(rg.Groups[group], func(i, j int) bool {
    return rg.Groups[group][i].Priority < rg.Groups[group][j].Priority
  })
  return nil
}

func (rg *RuleGroups) Remove(group string, ids ...string) error {
  if group == "" {
    return ErrInvalidArgs
  }
  rg.Lock()
  defer rg.Unlock()
  if _, ok := rg.Groups[group]; !ok {
    return ErrGroupNotFound
  }
  if len(ids) == 0 {
    delete(rg.Groups, group)
    return nil
  }
  for _, id := range ids {
    index := -1
    for i, r := range rg.Groups[group] {
      if id == r.Id {
        index = i
        break
      }
    }
    if index != -1 {
      rg.Groups[group] = append(rg.Groups[group][:index], rg.Groups[group][index+1:]...)
    }
  }
  return nil
}

func initRule(rule *Rule) {
  if rule.Group == "" {
    rule.Group = "default"
  }
  rule.patterns = make([]*Pattern, 0, len(rule.Patterns))
  for _, p := range rule.Patterns {
    re, e := regexp.Compile(p)
    if e != nil {
      continue
    }
    rule.patterns = append(rule.patterns, &Pattern{p, re})
  }
  if rule.Prepare != nil && rule.Prepare.Wait != "" {
    rule.Prepare.wait, _ = time.ParseDuration(rule.Prepare.Wait)
  }
  rule.timeout = time.Second * 10
  if rule.Timeout != "" {
    rule.timeout, _ = time.ParseDuration(rule.Timeout)
  }
  for _, f := range rule.Fields {
    if f.Wait != "" {
      f.wait, _ = time.ParseDuration(f.Wait)
    }
  }
  if rule.Loop != nil {
    if rule.Loop.ExportCycle == 0 {
      rule.Loop.ExportCycle = 10
    }
    if rule.Loop.Prepare != nil && rule.Loop.Prepare.Wait != "" {
      rule.Loop.Prepare.wait, _ = time.ParseDuration(rule.Loop.Prepare.Wait)
    }
    if rule.Loop.Wait != "" {
      rule.Loop.wait, _ = time.ParseDuration(rule.Loop.Wait)
    }
  }
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
