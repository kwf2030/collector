package collector

import (
  "encoding/json"
  "fmt"
  "runtime"
  "strconv"
  "sync"
  "testing"
  "time"

  "github.com/kwf2030/cdp"
)

var wg3 sync.WaitGroup

var listRule = []byte(`id: "1"
version: 1
name: "1"
group: "group1"
priority: 100

patterns:
  - "mohurd.gov.cn"

timeout: "300s"

loop:
  name: "url"
  alias: "链接"
  export_cycle: 1
  eval: "{let ret=[];let arr=Array.prototype.slice.call(document.querySelectorAll('.primary'));for (let i=0;i<arr.length;i++) {ret[i]=arr[i].firstElementChild.href;}JSON.stringify(ret);}"
  next: "{Array.prototype.slice.call(document.querySelector('.quotes').children).filter(e => {return e.textContent.trim()==(cdp_loop_count+1).toString()})[0].click();true}"
  wait: "5s"
`)

var detailRule = []byte(`id: "2"
version: 1
name: "2"
group: "group2"
priority: 100

patterns:
  - "mohurd.gov.cn"

timeout: "300s"

fields:
  - name: "name"
    alias: "企业名称"
    eval: "document.querySelector('.fa-building-o').nextSibling.textContent.trim()"
    export: true

  - name: "code"
    alias: "组织机构代码/营业执照编号"
    eval: "document.querySelector('body > div.main_box.nav_mtop > div:nth-child(2) > div > table > tbody > tr:nth-child(1) > td').textContent.trim()"
    export: true

  - name: "ceo"
    alias: "企业法人"
    eval: "document.querySelector('body > div.main_box.nav_mtop > div:nth-child(2) > div > table > tbody > tr:nth-child(2) > td:nth-child(2)').textContent.trim()"
    export: true

  - name: "province"
    alias: "企业注册属地"
    eval: "document.querySelector('body > div.main_box.nav_mtop > div:nth-child(2) > div > table > tbody > tr:nth-child(3) > td').textContent.trim()"
    export: true

  - name: "qualification_type"
    alias: "资质类别"
    eval: "document.querySelector('.row').children[1].textContent.trim()"
    export: true

  - name: "qualification_code"
    alias: "资质编号"
    eval: "document.querySelector('.row').children[2].textContent.trim()"
    export: true

  - name: "qualification_name"
    alias: "资质名称"
    eval: "document.querySelector('.row').children[3].textContent.trim()"
    export: true
`)

var (
  chrome     *cdp.Chrome
  ruleGroup1 *RuleGroup
  ruleGroup2 *RuleGroup
)

func TestJZJG(t *testing.T) {
  bin := ""
  switch runtime.GOOS {
  case "windows":
    bin = "C:/Program Files (x86)/Google/Chrome/Application/chrome.exe"
  case "linux":
    bin = "/usr/bin/google-chrome-stable"
  }
  chrome, _ = cdp.Launch(bin)

  ruleGroup1 = NewRuleGroup("group1")
  e := ruleGroup1.AppendBytes(listRule)
  if e != nil {
    t.Fatal(e)
  }
  ruleGroup2 = NewRuleGroup("group2")
  e = ruleGroup2.AppendBytes(detailRule)
  if e != nil {
    panic(e)
  }

  p := NewPage("list", "http://jzsc.mohurd.gov.cn/dataservice/query/comp/list", "group1")
  e = p.Collect(chrome, ruleGroup1, &OrgList{})
  if e != nil {
    t.Fatal(e)
  }
  wg3.Add(1)
  wg3.Wait()
  chrome.Exit()
}

type OrgList struct{}

func (s *OrgList) OnFields(p *Page, data map[string]string) {
}

func (s *OrgList) OnLoop(p *Page, loopCount int, data []string) bool {
  var arr []string
  e := json.Unmarshal([]byte(data[0]), &arr)
  if e != nil {
    panic(e)
  }
  fmt.Printf("====================第%d页（%d家企业）\n", loopCount, len(arr))
  for i, v := range arr {
    w := &sync.WaitGroup{}
    w.Add(1)
    crawlOrg(w, i, v)
    w.Wait()
    time.Sleep(time.Millisecond * 500)
  }
  return true
}

func (s *OrgList) OnComplete(p *Page) {
  wg3.Done()
}

type OrgDetail struct {
  w *sync.WaitGroup
}

func (s *OrgDetail) OnFields(p *Page, data map[string]string) {
  for _, f := range p.Rule.Fields {
    fmt.Println(f.Alias, ":", data[f.Name])
  }
}

func (s *OrgDetail) OnLoop(p *Page, loopCount int, data []string) bool {
  return true
}

func (s *OrgDetail) OnComplete(p *Page) {
  fmt.Println()
  p.Close()
  s.w.Done()
}

func crawlOrg(w *sync.WaitGroup, id int, url string) {
  if url == "" {
    return
  }
  fmt.Println("正在采集", id, url)
  p := NewPage(strconv.Itoa(id), url, "group2")
  e := p.Collect(chrome, ruleGroup2, &OrgDetail{w})
  if e != nil {
    panic(e)
  }
}
