package step_based_workflow

import "fmt"

// The mode gate must agree with execute_step's published schema. Step type and
// saved-script existence are checked later by the normal execution path.
func validateWorkshopFastPathRequest(mode string, requested bool) error {
	if requested && mode != "workshop" {
		return fmt.Errorf("fast_path_only is available only in Workshop mode for saved-script testing; in Run mode use execute_step without fast_path_only")
	}
	return nil
}
