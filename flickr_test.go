// Copyright 2011 Muthukannan T <manki@manki.in>. All Rights Reserved.

package flickgo

import (
  "bytes"
  "crypto/md5"
  "fmt"
  "hash"
  "http"
  "io"
  "io/ioutil"
  "os"
  "strings"
  "testing"
)

const (
  apiKey = "87337fd784"
  secret = "sf97838dijd"
)


func assert(t *testing.T, tag string, cond bool) {
  if !cond {
    t.Errorf("[%s] assertion failed", tag)
  }
}

func assertOK(t *testing.T, id string, err os.Error) {
  if err != nil {
    t.Errorf("[%s] unexpcted error: %v", id, err)
  }
}

func assertEq(t *testing.T, id string, expected interface{}, actual interface{}) {
  if expected != actual {
    t.Errorf("[%s] expcted: <%v>, found <%v>", id, expected, actual)
  }
}


//-----------------------
// Tests for request.go
//
func write(h hash.Hash, s string) {
  h.Write([]byte(s))
}

func TestSignedURL(t *testing.T) {
  m := md5.New()
  write(m, secret)
  write(m, "123" + "98765")
  write(m, "abc" + "abc+def")
  write(m, "api_key" + "apap983+key")
  write(m, "xyz" + "xyz")
  sig := fmt.Sprintf("%x", m.Sum())

  args := map[string]string{
      "abc": "abc def",
      "xyz": "xyz",
      "123": "98765",
      }
  argsMm := map[string][]string{
      "abc": []string{"abc def"},
      "xyz": []string{"xyz"},
      "123": []string{"98765"},
      "api_key": []string{"apap983 key"},
      "api_sig": []string{sig},
      }

  expected := "http://www.flickr.com/services/srv/?" + http.EncodeQuery(argsMm)
  actual := signedURL(secret, "apap983 key", "srv", args)
  assertEq(t, "url", expected, actual)
}


type fakeBody struct {
  error os.Error
  data []byte
  read bool
}
func (f fakeBody) Read(buf []byte) (int, os.Error) {
  if (currentBody.read) {
    return 0, os.EOF
  }

  for i, b := range f.data {
    buf[i] = b
  }
  currentBody.read = true
  return len(f.data), f.error
}
func (f fakeBody) Close() os.Error {
  return nil
}

// "Methods" of fakeBody take a fakeBody instance _by value_, which means they
// cannot mutate the instance being operated on.  This global reference will be
// set by tests and mutated by fakeBody's methods.  Big time facepalm!
var currentBody fakeBody

type fakeRoundTripper struct {
  err os.Error
  body fakeBody
  getFn func(r *http.Request) (*http.Response, os.Error)
}
func (f fakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, os.Error) {
  return f.getFn(r)
}

func newHTTPClient(getFn func(*http.Request) (*http.Response, os.Error)) *http.Client {
  rt := fakeRoundTripper{getFn: getFn}
  return &http.Client{Transport: rt}
}

func TestFetchHttpGetFails(t *testing.T) {
  url := "http://some.url/?arg=value"
  err := os.NewError("random error")
  getFn := func(r *http.Request) (*http.Response, os.Error) {
    assertEq(t, "url", url, r.URL.String())
    return nil, err
  }
  c := New(apiKey, secret, newHTTPClient(getFn))

  resp, e := fetch(c, url)
  assert(t, "resp", resp == nil)
  assertEq(t, "err", fmt.Sprintf("GET failed: Get %s: %s", url, err), e.String())
}

func TestFetchSuccess(t *testing.T) {
  url := "http://some.url/?arg=value"

  expectedData := bytes.NewBufferString("response from Flickr").Bytes()
  body := fakeBody{data: expectedData}
  currentBody = body
  resp := http.Response{Body: body}
  getFn := func(r *http.Request) (*http.Response, os.Error) {
    assertEq(t, "url", url, r.URL.String())
    return &resp, nil
  }
  c := New(apiKey, secret, newHTTPClient(getFn))

  in, e := fetch(c, url)
  assertOK(t, "fetch", e)
  buf := bytes.NewBuffer(nil)
  _, cErr := io.Copy(buf, in)
  assertOK(t, "copy", cErr)
  assert(t, "data", bytes.Equal(expectedData, buf.Bytes()))
}

func TestUploadRequest(t *testing.T) {
  data := []byte("123456\n78910\nasdfoiu\nasdfeejh")
  filename := "kitten.JPEG"
  args := map[string]string{
      "title": "kitten",
      "description": "my cute kitten",
      }
  authToken := "ase878723623"
  c := New(apiKey, secret, nil)
  c.AuthToken = authToken
  req, rqErr := uploadRequest(c, filename, data, args)
  assertOK(t, "uploadRequest", rqErr)
  pErr := req.ParseMultipartForm(128)
  assertOK(t, "parseForm", pErr)

  args["api_key"] = apiKey
  args["auth_token"] = authToken
  args["async"] = "1"
  apiSig := sign(secret, args)

  form := req.MultipartForm
  verify := func(key, value string) {
    assertEq(t, key + " len", 1, len(form.Value[key]))
    assertEq(t, key, value, form.Value[key][0])
  }
  assertEq(t, "value len", 6, len(form.Value))
  verify("title", "kitten")
  verify("description", "my cute kitten")
  verify("api_key", apiKey)
  verify("auth_token", authToken)
  verify("api_sig", apiSig)
  verify("async", "1")

  assertEq(t, "file len", 1, len(form.File))
  assertEq(t, "photo len", 1, len(form.File["photo"]))
  assertEq(t, "filename", filename, form.File["photo"][0].Filename)
  assertEq(t, "filetype", "image/jpeg",
           form.File["photo"][0].Header.Get("Content-Type"))
  file, oErr := form.File["photo"][0].Open()
  assertOK(t, "file open", oErr)
  actual, rdErr := ioutil.ReadAll(file)
  assertOK(t, "read file", rdErr)
  assertEq(t, "photo", string(data), string(actual))
}


//-----------------------
// Tests for flickr.go
//
func TestAuthURL(t *testing.T) {
  c := New(apiKey, secret, nil)

  u, uErr := http.ParseURL(c.AuthURL(ReadPerm))
  assertOK(t, "parseURL", uErr)
  args, qErr := http.ParseQuery(u.RawQuery)
  assertOK(t, "parseQuery", qErr)

  for _, key := range []string{"api_key", "perms", "api_sig"} {
    if (len(args[key]) != 1) {
      t.Errorf("Query argument %s has value %v", key, args[key])
    }
  }
  assertEq(t, "api_key", apiKey, args["api_key"][0])
  assertEq(t, "perms", ReadPerm, args["perms"][0])
}

func TestGetTokenURL(t *testing.T) {
  frob := "837cjnei"
  c := New(apiKey, secret, nil)

  u, uErr := http.ParseURL(getTokenURL(c, frob))
  assertOK(t, "parseURL", uErr)
  args, err := http.ParseQuery(u.RawQuery)
  assertOK(t, "parseQuery", err)
  assertEq(t, "method", "flickr.auth.getToken", args["method"][0])
  assertEq(t, "frob", frob, args["frob"][0])
  assertEq(t, "api_key", apiKey, args["api_key"][0])
  assertEq(t, "api_sig", 1, len(args["api_sig"]))
}

func TestGetTokenAPIFailure(t *testing.T) {
  xmlStr := `<?xml version="1.0" encoding="utf-8"?>
    <rsp stat="fail">
      <err code="97" msg="Missing signature"/>
    </rsp>`
  xmlBytes := bytes.NewBufferString(xmlStr).Bytes()
  body := fakeBody{data: xmlBytes}
  currentBody = body
  resp := http.Response{Body: body}
  getFn := func(r *http.Request) (*http.Response, os.Error) {
    return &resp, nil
  }
  c := New(apiKey, secret, newHTTPClient(getFn))
  _, err := c.GetToken("878243")
  assert(t, "err", err != nil)
  assert(t, "message: " + err.String(),
         strings.Contains(err.String(), "code 97: Missing signature"))
}

func TestGetToken(t *testing.T) {
  xmlStr := `<?xml version="1.0" encoding="utf-8"?>
    <rsp stat="ok">
      <auth>
        <token>121-84669832774</token>
        <perms>write</perms>
        <user nsid="7687633@N01" username="testuser" fullname="Test User"/>
      </auth>
    </rsp>`
  xmlBytes := bytes.NewBufferString(xmlStr).Bytes()
  body := fakeBody{data: xmlBytes}
  currentBody = body
  resp := http.Response{Body: body}
  getFn := func(r *http.Request) (*http.Response, os.Error) {
    return &resp, nil
  }
  c := New(apiKey, secret, newHTTPClient(getFn))
  tok, err := c.GetToken("878243")
  assertOK(t, "GetToken", err)
  assertEq(t, "token", "121-84669832774", tok)
}

func TestUploadFails(t *testing.T) {
  xmlStr := `<?xml version="1.0" encoding="utf-8"?>
    <rsp stat="fail">
      <err code="5" msg="Filetype was not recognised"/>
    </rsp>`
  xmlBytes := bytes.NewBufferString(xmlStr).Bytes()
  body := fakeBody{data: xmlBytes}
  currentBody = body
  resp := http.Response{Body: body}
  postFn := func(r *http.Request) (*http.Response, os.Error) {
    return &resp, nil
  }
  c := New(apiKey, secret, newHTTPClient(postFn))
  ticket, err := c.Upload("filename", []byte("photo content"),
                          map[string]string{})
  assert(t, "message: " + err.String(),
         strings.Contains(err.String(), "code 5: Filetype was not recognised"))
  assertEq(t, "ticket", "", ticket)
}

func TestUpload(t *testing.T) {
  xmlStr := `<?xml version="1.0" encoding="utf-8"?>
    <rsp stat="ok">
      <ticketid>363</ticketid>
    </rsp>`
  xmlBytes := bytes.NewBufferString(xmlStr).Bytes()
  body := fakeBody{data: xmlBytes}
  currentBody = body
  resp := http.Response{Body: body}
  postFn := func(r *http.Request) (*http.Response, os.Error) {
    return &resp, nil
  }
  c := New(apiKey, secret, newHTTPClient(postFn))
  ticket, err := c.Upload("filename", make([]byte, 1024 * 1024),
                          map[string]string{})
  assertOK(t, "upload", err)
  assertEq(t, "ticket", "363", ticket)
}

func TestSearchURL(t *testing.T) {
  args := map[string]string{
      "per_page": "10",
      "user_id": "me",
      }
  c := New(apiKey, secret, nil)

  u, uErr := http.ParseURL(searchURL(c, args))
  assertOK(t, "parseURL", uErr)
  a, err := http.ParseQuery(u.RawQuery)
  assertOK(t, "parseQuery", err)
  assertEq(t, "method", "flickr.photos.search", a["method"][0])
  assertEq(t, "per_page", "10", a["per_page"][0])
  assertEq(t, "user_id", "me", a["user_id"][0])
  assertEq(t, "api_key", apiKey, a["api_key"][0])
  assertEq(t, "api_sig", 1, len(a["api_sig"]))
}

func TestSearch(t *testing.T) {
  xmlStr := `<?xml version="1.0" encoding="utf-8"?>
    <rsp stat="ok">
      <photos page="1" pages="3" perpage="2" total="5">
      <photo id="1234" owner="22@N01" secret="63562" server="3" farm="1"
             title="kitten" ispublic="0" isfriend="1" isfamily="1"/>
      <photo id="5678" owner="22@N01" secret="36221" server="32" farm="4"
             title="puppies" ispublic="0" isfriend="0" isfamily="0"/>
      </photos>
    </rsp>`
  xmlBytes := bytes.NewBufferString(xmlStr).Bytes()
  body := fakeBody{data: xmlBytes}
  currentBody = body
  resp := http.Response{Body: body}
  getFn := func(r *http.Request) (*http.Response, os.Error) {
    return &resp, nil
  }
  c := New(apiKey, secret, newHTTPClient(getFn))
  r, err := c.Search(map[string]string{})
  assertOK(t, "search", err)
  assertEq(t, "page", "1", r.Page)
  assertEq(t, "pages", "3", r.Pages)
  assertEq(t, "perpage", "2", r.PerPage)
  assertEq(t, "total", "5", r.Total)
  assertEq(t, "len photos", 2, len(r.Photos))

  verify := func(p Photo, idx int,
                 id, owner, secret, server, farm, title string) {
    assertEq(t, fmt.Sprintf("%d.id", idx), id, p.Id)
    assertEq(t, fmt.Sprintf("%d.owner", idx), owner, p.Owner)
    assertEq(t, fmt.Sprintf("%d.secret", idx), secret, p.Secret)
    assertEq(t, fmt.Sprintf("%d.server", idx), server, p.Server)
    assertEq(t, fmt.Sprintf("%d.farm", idx), farm, p.Farm)
    assertEq(t, fmt.Sprintf("%d.title", idx), title, p.Title)
  }
  verify(r.Photos[0], 0, "1234", "22@N01", "63562", "3", "1", "kitten")
  verify(r.Photos[1], 1, "5678", "22@N01", "36221", "32", "4", "puppies")
}

func TestURL(t *testing.T) {
  p := Photo{
    Id: "id",
    Owner: "owner",
    Secret: "secret",
    Server: "server",
    Farm: "fx",
    Title: "title",
  }
  assertEq(t, "url", "http://farmfx.static.flickr.com/server/id_secret_-.jpg",
           p.URL(SizeMedium500))
}
