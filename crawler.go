package crawler

import (
  "errors"
  "fmt"
  "html"
  "runtime"
  "strconv"
  "sync"
  "time"

  "github.com/kwf2030/cdp"
  "github.com/kwf2030/commons/conv"
)

var (
  ErrInvalidArgs   = errors.New("invalid args")
  ErrNoRuleMatched = errors.New("no rule matched")

  chrome *cdp.Chrome
)

type Handler interface {
  OnFields(*Page, map[string]string)

  OnLoop(*Page, int, []string)

  OnComplete(*Page)
}

type Page struct {
  // 页面Id（与规则Id无关）
  Id string

  Url string

  // 在该规则分组下匹配规则
  Group string

  rule *Rule

  tab *cdp.Tab

  handler Handler

  once sync.Once
}

func NewPage(id, url, group string) *Page {
  if url == "" {
    return nil
  }
  return &Page{Id: id, Url: url, Group: group}
}

func (p *Page) OnCdpEvent(msg *cdp.Message) {
  if msg.Method == cdp.Page.LoadEventFired {
    // 如果超时，就有可能存在两次回调（超时一次回调和正常一次回调），
    // once是为了防止重复调用
    p.once.Do(func() {
      m := p.crawlFields()
      if p.handler != nil {
        p.handler.OnFields(p, m)
      }
      if p.rule.Loop != nil {
        p.crawlLoop()
      }
      if p.handler != nil {
        p.handler.OnComplete(p)
      }
    })
  }
}

func (p *Page) OnCdpResp(msg *cdp.Message) bool {
  return false
}

func (p *Page) Crawl(h Handler) error {
  if p.Url == "" {
    return ErrInvalidArgs
  }
  if p.Group == "" {
    p.Group = "default"
  }
  addr := html.UnescapeString(p.Url)
  rule := Rules.match(p.Group, addr)
  if rule == nil {
    return ErrNoRuleMatched
  }
  tab, e := chrome.NewTab(p)
  if e != nil {
    return e
  }
  p.rule = rule
  p.tab = tab
  p.handler = h
  tab.Subscribe(cdp.Page.LoadEventFired)
  tab.Call(cdp.Page.Enable, nil)
  tab.Call(cdp.Page.Navigate, cdp.Param{"url": addr})
  // todo 大量定时器，如果有性能问题改用时间轮
  time.AfterFunc(p.rule.timeout, func() {
    tab.FireEvent(cdp.Page.LoadEventFired, nil)
  })
  return nil
}

func (p *Page) crawlFields() map[string]string {
  rule := p.rule
  ret := make(map[string]string, len(rule.Fields))
  params := cdp.Param{"objectGroup": "console", "includeCommandLineAPI": true}
  if rule.Prepare != nil {
    if rule.Prepare.Eval != "" {
      if rule.Prepare.Eval[0] == '{' {
        params["expression"] = rule.Prepare.Eval
      } else {
        params["expression"] = "{" + rule.Prepare.Eval + "}"
      }
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := conv.GetString(conv.GetMap(msg.Result, "result"), "value", "false")
      if r != "true" {
        return ret
      }
    }
    if rule.Prepare.wait > 0 {
      time.Sleep(rule.Prepare.wait)
    }
  }
  for _, field := range rule.Fields {
    if field.Eval != "" {
      if field.Value != "" {
        params["expression"] = fmt.Sprintf("{let value='%s';%s}", field.Value, field.Eval)
      } else {
        if field.Eval[0] == '{' {
          params["expression"] = field.Eval
        } else {
          params["expression"] = "{" + field.Eval + "}"
        }
      }
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := conv.GetString(conv.GetMap(msg.Result, "result"), "value", "")
      ret[field.Name] = r
      params["expression"] = fmt.Sprintf("const %s='%s'", field.Name, r)
      p.tab.Call(cdp.Runtime.Evaluate, params)
    } else if field.Value != "" {
      ret[field.Name] = field.Value
      params["expression"] = fmt.Sprintf("const %s='%s'", field.Name, field.Value)
      p.tab.Call(cdp.Runtime.Evaluate, params)
    }
    if field.wait > 0 {
      time.Sleep(field.wait)
    }
  }
  return ret
}

func (p *Page) crawlLoop() {
  rule := p.rule
  params := cdp.Param{"objectGroup": "console", "includeCommandLineAPI": true}
  if rule.Loop.Prepare != nil {
    if rule.Loop.Prepare.Eval != "" {
      if rule.Loop.Prepare.Eval[0] == '{' {
        params["expression"] = rule.Loop.Prepare.Eval
      } else {
        params["expression"] = "{" + rule.Loop.Prepare.Eval + "}"
      }
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := conv.GetString(conv.GetMap(msg.Result, "result"), "value", "false")
      if r != "true" {
        return
      }
    }
    if rule.Loop.Prepare.wait > 0 {
      time.Sleep(rule.Loop.Prepare.wait)
    }
  }
  if rule.Loop.Eval != "" && rule.Loop.Eval[0] != '{' {
    rule.Loop.Eval = "{" + rule.Loop.Eval + "}"
  }
  if rule.Loop.Next != "" && rule.Loop.Next[0] != '{' {
    rule.Loop.Next = "{" + rule.Loop.Next + "}"
  }
  i := 0
  arr := make([]string, rule.Loop.ExportCycle)
  for {
    i++
    // eval
    if rule.Loop.Eval != "" {
      params["expression"] = rule.Loop.Eval
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := conv.GetString(conv.GetMap(msg.Result, "result"), "value", "")
      exp := "count=" + strconv.Itoa(i) + ";"
      if i == 1 {
        exp = "let " + exp
      }
      params["expression"] = exp
      p.tab.Call(cdp.Runtime.Evaluate, params)
      n := i % rule.Loop.ExportCycle
      if n != 0 {
        arr[n-1] = r
      } else {
        arr[rule.Loop.ExportCycle-1] = r
        if p.handler != nil {
          p.handler.OnLoop(p, i, arr)
        }
      }
    }
    // next
    if rule.Loop.Next != "" {
      params["expression"] = rule.Loop.Next
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := conv.GetString(conv.GetMap(msg.Result, "result"), "value", "")
      if r != "true" {
        return
      }
    }
    // wait
    if rule.Loop.wait > 0 {
      time.Sleep(rule.Loop.wait)
    }
  }
}

func LaunchChrome(bin string, args ...string) error {
  if bin == "" {
    switch runtime.GOOS {
    case "windows":
      bin = "C:/Program Files (x86)/Google/Chrome/Application/chrome.exe"
    case "linux":
      bin = "/usr/bin/google-chrome-stable"
    }
  }
  var e error
  chrome, e = cdp.Launch(bin, args...)
  if e != nil {
    return e
  }
  return nil
}

func ExitChrome() {
  if chrome != nil {
    chrome.Exit()
  }
}
