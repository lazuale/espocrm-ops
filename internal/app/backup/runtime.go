package backup

import (
	"github.com/lazuale/espocrm-ops/internal/app/ports"
	domainruntime "github.com/lazuale/espocrm-ops/internal/domain/runtime"
)

type runtimePrepareInfo struct {
	AppServicesWereRunning bool
	StoppedAppServices     []string
}

type runtimeReturnInfo struct {
	RestartedAppServices []string
}

func (s Service) prepareRuntime(target ports.RuntimeTarget) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	runningServices, err := s.runtime.RunningServices(target)
	if err != nil {
		return info, err
	}

	info.AppServicesWereRunning = domainruntime.AppServicesRunning(runningServices)
	info.StoppedAppServices = domainruntime.RunningAppServices(runningServices)
	if len(info.StoppedAppServices) == 0 {
		return info, nil
	}

	if err := s.runtime.Stop(target, info.StoppedAppServices...); err != nil {
		return info, err
	}

	return info, nil
}

func (s Service) returnRuntime(target ports.RuntimeTarget, prep runtimePrepareInfo) (runtimeReturnInfo, error) {
	info := runtimeReturnInfo{}
	if len(prep.StoppedAppServices) == 0 {
		return info, nil
	}

	if err := s.runtime.Up(target, prep.StoppedAppServices...); err != nil {
		return info, err
	}

	info.RestartedAppServices = append(info.RestartedAppServices, prep.StoppedAppServices...)
	return info, nil
}
