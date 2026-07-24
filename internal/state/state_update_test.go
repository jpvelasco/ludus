package state

import "testing"

func TestFleet(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		setupTest(t)

		if err := UpdateFleet(&FleetState{FleetID: "f-1", Status: "active"}); err != nil {
			t.Fatalf("UpdateFleet: %v", err)
		}
		s := mustLoad(t)
		if s.Fleet == nil || s.Fleet.FleetID != "f-1" {
			t.Fatal("fleet not updated")
		}
	})

	t.Run("clear also clears session", func(t *testing.T) {
		setupTest(t)

		if err := UpdateFleet(&FleetState{FleetID: "f-1", Status: "active"}); err != nil {
			t.Fatal(err)
		}
		if err := ClearFleet(); err != nil {
			t.Fatalf("ClearFleet: %v", err)
		}
		s := mustLoad(t)
		if s.Fleet != nil {
			t.Error("fleet should be nil after clear")
		}
		if s.Session != nil {
			t.Error("session should also be cleared with fleet")
		}
	})
}

func TestSession(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		setupTest(t)

		if err := UpdateSession(&SessionState{SessionID: "s-1", IPAddress: "1.2.3.4", Port: 7777}); err != nil {
			t.Fatalf("UpdateSession: %v", err)
		}
		s := mustLoad(t)
		if s.Session == nil || s.Session.SessionID != "s-1" {
			t.Fatal("session not updated")
		}
	})

	t.Run("clear", func(t *testing.T) {
		setupTest(t)

		if err := UpdateSession(&SessionState{SessionID: "s-1", IPAddress: "1.2.3.4", Port: 7777}); err != nil {
			t.Fatal(err)
		}
		if err := ClearSession(); err != nil {
			t.Fatalf("ClearSession: %v", err)
		}
		if s := mustLoad(t); s.Session != nil {
			t.Error("session should be nil after clear")
		}
	})
}

func TestEC2Fleet(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		setupTest(t)

		if err := UpdateEC2Fleet(&EC2FleetState{
			FleetID:  "ec2-fleet-1",
			BuildID:  "build-1",
			S3Bucket: "my-bucket",
			Status:   "active",
		}); err != nil {
			t.Fatal(err)
		}
		s := mustLoad(t)
		if s.EC2Fleet == nil || s.EC2Fleet.FleetID != "ec2-fleet-1" {
			t.Fatal("EC2 fleet not updated")
		}
	})

	t.Run("clear", func(t *testing.T) {
		setupTest(t)

		if err := UpdateEC2Fleet(&EC2FleetState{FleetID: "ec2-fleet-1", Status: "active"}); err != nil {
			t.Fatal(err)
		}
		if err := ClearEC2Fleet(); err != nil {
			t.Fatal(err)
		}
		if s := mustLoad(t); s.EC2Fleet != nil {
			t.Error("EC2 fleet should be nil after clear")
		}
	})
}

func TestAnywhere(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		setupTest(t)

		if err := UpdateAnywhere(&AnywhereState{
			FleetID:    "anywhere-1",
			IPAddress:  "192.168.1.1",
			ServerPort: 7777,
		}); err != nil {
			t.Fatal(err)
		}
		s := mustLoad(t)
		if s.Anywhere == nil || s.Anywhere.FleetID != "anywhere-1" {
			t.Fatal("anywhere not updated")
		}
	})

	t.Run("clear", func(t *testing.T) {
		setupTest(t)

		if err := UpdateAnywhere(&AnywhereState{FleetID: "anywhere-1"}); err != nil {
			t.Fatal(err)
		}
		if err := ClearAnywhere(); err != nil {
			t.Fatal(err)
		}
		if s := mustLoad(t); s.Anywhere != nil {
			t.Error("anywhere should be nil after clear")
		}
	})
}

func TestUpdateDeploy(t *testing.T) {
	setupTest(t)

	if err := UpdateDeploy(&DeployState{
		TargetName: "gamelift",
		Status:     "active",
		Detail:     "fleet-abc",
	}); err != nil {
		t.Fatal(err)
	}
	s := mustLoad(t)
	if s.Deploy == nil {
		t.Fatal("expected deploy state")
	}
	if s.Deploy.TargetName != "gamelift" {
		t.Errorf("target name: got %q, want %q", s.Deploy.TargetName, "gamelift")
	}
}

func TestUpdateEngineImage(t *testing.T) {
	setupTest(t)

	if err := UpdateEngineImage(&EngineImageState{
		ImageTag: "ludus-engine:5.7",
		Version:  "5.7",
		BuiltAt:  "2025-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	s := mustLoad(t)
	if s.EngineImage == nil || s.EngineImage.ImageTag != "ludus-engine:5.7" {
		t.Fatal("engine image not updated")
	}
}

func TestUpdateClient(t *testing.T) {
	setupTest(t)

	if err := UpdateClient(&ClientState{
		BinaryPath: "/path/to/client",
		Platform:   "Win64",
		BuiltAt:    "2025-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	s := mustLoad(t)
	if s.Client == nil || s.Client.Platform != "Win64" {
		t.Fatal("client not updated")
	}
}

func TestWSL2Engine(t *testing.T) {
	setupTest(t)
	want := &WSL2EngineState{
		EnginePath: "~/ludus/engine/5.8",
		IsNative:   true,
		DDCPath:    "~/ludus/ddc",
		SyncTime:   "2026-07-24T01:02:03Z",
		BuiltAt:    "2026-07-24T04:05:06Z",
	}
	if err := UpdateWSL2Engine(want); err != nil {
		t.Fatalf("UpdateWSL2Engine: %v", err)
	}
	if got := mustLoad(t).WSL2Engine; got == nil || *got != *want {
		t.Fatalf("WSL2 engine = %#v, want %#v", got, want)
	}
	if err := ClearWSL2Engine(); err != nil {
		t.Fatalf("ClearWSL2Engine: %v", err)
	}
	if got := mustLoad(t).WSL2Engine; got != nil {
		t.Errorf("WSL2 engine = %#v after clear, want nil", got)
	}
}
