package auth

import "testing"

func TestSignIsStable(t *testing.T) {
	in := testInput()
	first := Sign([]byte("secret"), in)
	second := Sign([]byte("secret"), in)

	if first == "" {
		t.Fatal("Sign() returned empty signature")
	}
	if first != second {
		t.Fatalf("signature is not stable: %q != %q", first, second)
	}
	if !Verify([]byte("secret"), in, first) {
		t.Fatal("Verify() should accept matching signature")
	}
}

func TestBodyChangeChangesSignature(t *testing.T) {
	in := testInput()
	sig := Sign([]byte("secret"), in)

	in.Body = []byte(`{"command_id":"cmd_2"}`)
	if Verify([]byte("secret"), in, sig) {
		t.Fatal("Verify() accepted tampered body")
	}
}

func TestSecretChangeFailsVerify(t *testing.T) {
	in := testInput()
	sig := Sign([]byte("secret-a"), in)

	if Verify([]byte("secret-b"), in, sig) {
		t.Fatal("Verify() accepted signature with different secret")
	}
}

func TestCanonicalFieldsAffectSignature(t *testing.T) {
	base := testInput()
	sig := Sign([]byte("secret"), base)

	cases := map[string]func(SignInput) SignInput{
		"method": func(in SignInput) SignInput {
			in.Method = "GET"
			return in
		},
		"path": func(in SignInput) SignInput {
			in.Path = "/api/agent/v1/other"
			return in
		},
		"timestamp": func(in SignInput) SignInput {
			in.Timestamp = "2026-06-23T00:01:00Z"
			return in
		},
		"nonce": func(in SignInput) SignInput {
			in.Nonce = "nonce-other"
			return in
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if Verify([]byte("secret"), mutate(base), sig) {
				t.Fatalf("Verify() accepted changed %s", name)
			}
		})
	}
}

func TestBodySHA256(t *testing.T) {
	got := BodySHA256([]byte("abc"))
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("BodySHA256() = %q, want %q", got, want)
	}
}

func testInput() SignInput {
	return SignInput{
		Method:    "POST",
		Path:      "/api/agent/v1/command",
		Timestamp: "2026-06-23T00:00:00Z",
		Nonce:     "nonce-001",
		Body:      []byte(`{"command_id":"cmd_1"}`),
	}
}
