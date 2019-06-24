package collector

import (
  "encoding/json"
  "fmt"
  "runtime"
  "sync"
  "testing"
  "time"

  "github.com/kwf2030/cdp"
)

var wg2 sync.WaitGroup

var rule2 = []byte(`id: "002024"
version: 1
name: "002024"
group: "default"
priority: 100

patterns:
  - "gu.qq.com"

timeout: "10s"

fields:
  - name: name
    eval: "document.querySelector('.title_bg').firstElementChild.textContent.trim()"
    export: true

  - name: code
    eval: "document.querySelector('.title_bg').lastElementChild.textContent.trim()"
    export: true

  - name: total_market_value
    eval: "document.querySelectorAll('.col-2')[0].children[1].children[2].lastElementChild.textContent.trim()"
    export: true

  - name: circulated_market_value
    eval: "document.querySelectorAll('.col-2')[0].children[1].children[3].lastElementChild.textContent.trim()"
    export: true

  - name: closing_price
    eval: "document.querySelectorAll('.col-2')[0].children[0].children[0].lastElementChild.textContent.trim()"
    export: true

  - name: opening_price
    eval: "document.querySelectorAll('.col-2')[0].children[0].children[1].lastElementChild.textContent.trim()"
    export: true

loop:
  name: "002024"
  export_cycle: 1
  eval: "let ret={};ret['price']=document.querySelectorAll('.col-1')[1].children[1].children[0].textContent.trim();ret['rising_falling']=document.querySelectorAll('.col-1')[1].children[1].children[1].children[0].textContent.trim();ret['max_price']=document.querySelectorAll('.col-2')[0].children[0].children[2].lastElementChild.textContent.trim();ret['min_price']=document.querySelectorAll('.col-2')[0].children[0].children[3].lastElementChild.textContent.trim();ret['amplitude']=document.querySelectorAll('.col-2')[0].children[2].children[2].lastElementChild.textContent.trim();ret['turnover']=document.querySelectorAll('.col-2')[0].children[2].children[0].lastElementChild.textContent.trim();ret['volumes1']=document.querySelectorAll('.col-2')[0].children[1].children[0].lastElementChild.textContent.trim();ret['volumes2']=document.querySelectorAll('.col-2')[0].children[1].children[1].lastElementChild.textContent.trim();ret['pe']=document.querySelectorAll('.col-2')[0].children[2].children[3].lastElementChild.textContent.trim();ret['pb']=document.querySelectorAll('.col-2')[0].children[2].children[1].lastElementChild.textContent.trim();JSON.stringify(ret);"
  next: "cdp_loop_count<=10"
  wait: "1s"
`)

type Stock struct{}

func (s *Stock) OnFields(p *Page, data map[string]string) {
  fmt.Printf("%-10s%-9s%-6s%-7s%-8s%-8s\n", "代码", "名称", "总市值", "流通市值", "昨日收盘价", "今日开盘价")
  fmt.Printf("%-12s%-7s%-8s%-10s%-13s%-12s\n", data["code"], data["name"], data["total_market_value"], data["circulated_market_value"], data["closing_price"], data["opening_price"])
  fmt.Println()
  fmt.Printf("%-8s%-6s%-6s%-6s%-5s%-5s%-6s%-7s%-6s%-6s%-6s\n", "时间", "价格", "涨跌", "最高", "最低", "振幅", "换手率", "成交量", "成交额", "市盈率", "市净率")
}

func (s *Stock) OnLoop(p *Page, loopCount int, data []string) bool {
  for _, v := range data {
    m := make(map[string]string, 8)
    e := json.Unmarshal([]byte(v), &m)
    if e != nil {
      panic(e)
    }
    fmt.Printf("%-10s%-8s%-8s%-8s%-7s%-7s%-9s%-8s%-8s%-9s%-8s\n", time.Now().Format("15:04:05"), m["price"], m["rising_falling"], m["max_price"], m["min_price"], m["amplitude"], m["turnover"], m["volumes1"], m["volumes2"], m["pe"], m["pb"])
  }
  return true
}

func (s *Stock) OnComplete(p *Page) {
  wg2.Done()
}

func TestStock(t *testing.T) {
  bin := ""
  switch runtime.GOOS {
  case "windows":
    bin = "C:/Program Files (x86)/Google/Chrome/Application/chrome.exe"
  case "linux":
    bin = "/usr/bin/google-chrome-stable"
  }
  chrome, e := cdp.Launch(bin)
  if e != nil {
    t.Fatal(e)
  }

  rg := NewRuleGroup("default")
  e = rg.AppendBytes(rule2)
  if e != nil {
    t.Fatal(e)
  }

  p := NewPage("01", "http://gu.qq.com/sz002024/gp", "default")
  e = p.Collect(chrome, rg, &Stock{})
  if e != nil {
    t.Fatal(e)
  }

  wg2.Add(1)
  wg2.Wait()
  chrome.Exit()
}
