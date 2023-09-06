package SpiceDbPoC

import (
	"fmt"
	"os"
	"testing"
)

var container *LocalSpiceDbContainer

func TestMain(m *testing.M) {
	factory := NewLocalSpiceDbContainerFactory()
	var err error
	container, err = factory.CreateContainer()

	if err != nil {
		fmt.Printf("Error initializing Docker container: %s", err)
		os.Exit(-1)
	}

	result := m.Run()

	container.Close()
	os.Exit(result)
}

// TODO: what are good test cases? look at schema and prbac

// e.g. this one (just a braindump-outline)
func TestUser_With_Patch_Read_Permission_Can_Access_Get(t *testing.T) {
	// TODO
}

// or this one (just a braindump-outline)
func TestUser_Without_Inventory_Read_Permission_Does_Not_See_Anything(t *testing.T) {
	// TODO
}
