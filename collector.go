package collector

import (
  "errors"
  "fmt"
  "html"
  "strconv"
  "sync"
  "time"

  "github.com/kwf2030/cdp"
  "github.com/kwf2030/commons/base"
)

var ErrNoRuleMatched = errors.New("no rule matched")

type Handler interface {
  // 只会回调一次，所有字段一次性返回
  OnFields(*Page, map[string]string)

  // 按设置的导出周期回调（如export_cycle=5表示5次循环回调1次），返回值表示是否继续循环
  OnLoop(*Page, int, []string) bool

  // 在OnFields和OnLoop完成后调用
  OnComplete(*Page)
}

type Page struct {
  // 页面Id（与规则Id无关）
  Id string

  Url string

  // 在该规则分组下匹配规则
  Group string

  Rule *Rule

  tab *cdp.Tab

  handler Handler

  once sync.Once
}

func NewPage(id, url, group string) *Page {
  if url == "" || group == "" {
    return nil
  }
  return &Page{Id: id, Url: url, Group: group}
}

func (p *Page) OnCdpEvent(msg *cdp.Message) {
  if msg.Method == cdp.Page.LoadEventFired {
    // 如果超时，就有可能存在两次回调（超时一次回调和正常一次回调），
    // once是为了防止重复调用
    p.once.Do(func() {
      m := p.collectFields()
      if p.handler != nil {
        p.handler.OnFields(p, m)
      }
      if p.Rule.Loop != nil {
        p.collectLoop()
      }
      if p.handler != nil {
        p.handler.OnComplete(p)
      }
    })
  }
}

func (p *Page) OnCdpResponse(msg *cdp.Message) bool {
  return false
}

func (p *Page) Close() {
  if p.tab != nil {
    p.tab.Close()
  }
}

func (p *Page) Collect(chrome *cdp.Chrome, rg *RuleGroup, h Handler) error {
  if p.Url == "" {
    return base.ErrInvalidArgument
  }
  addr := html.UnescapeString(p.Url)
  rule := rg.match(addr)
  if rule == nil {
    return ErrNoRuleMatched
  }
  tab, e := chrome.NewTab(p)
  if e != nil {
    return e
  }
  p.Rule = rule
  p.tab = tab
  p.handler = h
  tab.Subscribe(cdp.Page.LoadEventFired)
  tab.Call(cdp.Page.Enable, nil)
  tab.Call(cdp.Page.Navigate, map[string]interface{}{"url": addr})
  // todo 如果定时器数量很大会有性能问题（改用时间轮）
  time.AfterFunc(p.Rule.timeout, func() {
    tab.Fire(cdp.Page.LoadEventFired, nil)
  })
  return nil
}

func (p *Page) collectFields() map[string]string {
  rule := p.Rule
  ret := make(map[string]string, len(rule.Fields))
  params := map[string]interface{}{"objectGroup": "console", "includeCommandLineAPI": true}
  if rule.Prepare != nil {
    if rule.Prepare.Eval != "" {
      if rule.Prepare.Eval[0] == '{' {
        params["expression"] = rule.Prepare.Eval
      } else {
        params["expression"] = "{" + rule.Prepare.Eval + "}"
      }
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := getValue(msg.Result)
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
        params["expression"] = fmt.Sprintf("{let cdp_field_value='%s';%s}", field.Value, field.Eval)
      } else {
        if field.Eval[0] == '{' {
          params["expression"] = field.Eval
        } else {
          params["expression"] = "{" + field.Eval + "}"
        }
      }
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := getValue(msg.Result)
      ret[field.Name] = r
      params["expression"] = fmt.Sprintf("const cdp_field_%s='%s'", field.Name, r)
      p.tab.Call(cdp.Runtime.Evaluate, params)
    } else if field.Value != "" {
      ret[field.Name] = field.Value
      params["expression"] = fmt.Sprintf("const cdp_field_%s='%s'", field.Name, field.Value)
      p.tab.Call(cdp.Runtime.Evaluate, params)
    }
    if field.wait > 0 {
      time.Sleep(field.wait)
    }
  }
  return ret
}

func (p *Page) collectLoop() {
  rule := p.Rule
  params := map[string]interface{}{"objectGroup": "console", "includeCommandLineAPI": true}
  if rule.Loop.Prepare != nil {
    if rule.Loop.Prepare.Eval != "" {
      if rule.Loop.Prepare.Eval[0] == '{' {
        params["expression"] = rule.Loop.Prepare.Eval
      } else {
        params["expression"] = "{" + rule.Loop.Prepare.Eval + "}"
      }
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := getValue(msg.Result)
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
  params["expression"] = "let cdp_loop_count=1;"
  arr := make([]string, rule.Loop.ExportCycle)
  for {
    i++
    n := i % rule.Loop.ExportCycle
    if i > 1 {
      params["expression"] = "cdp_loop_count=" + strconv.Itoa(i) + ";"
    }
    p.tab.Call(cdp.Runtime.Evaluate, params)
    // eval
    if rule.Loop.Eval != "" {
      params["expression"] = rule.Loop.Eval
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := getValue(msg.Result)
      if n == 0 {
        arr[rule.Loop.ExportCycle-1] = r
      } else {
        arr[n-1] = r
      }
    }
    if n == 0 {
      if p.handler != nil {
        if ok := p.handler.OnLoop(p, i, arr); !ok {
          break
        }
      }
      for j := 0; j < rule.Loop.ExportCycle; j++ {
        arr[j] = ""
      }
    }
    // next
    if rule.Loop.Next != "" {
      params["expression"] = rule.Loop.Next
      _, ch := p.tab.Call(cdp.Runtime.Evaluate, params)
      msg := <-ch
      r := getValue(msg.Result)
      if r != "true" {
        if p.handler != nil && n != 0 {
          p.handler.OnLoop(p, i, arr[:n])
        }
        break
      }
    }
    // wait
    if rule.Loop.wait > 0 {
      time.Sleep(rule.Loop.wait)
    }
  }
}

func getValue(data map[string]interface{}) string {
  r, ok := data["result"]
  if !ok {
    return ""
  }
  m, ok := r.(map[string]interface{})
  if !ok {
    return ""
  }
  v, ok := m["value"]
  if !ok {
    return ""
  }
  switch ret := v.(type) {
  case string:
    return ret
  case bool:
    if ret {
      return "true"
    }
  }
  return ""
}
