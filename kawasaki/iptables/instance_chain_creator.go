package iptables

import (
	"fmt"
	"net"
	"os/exec"

	"code.cloudfoundry.org/lager"
)

type InstanceChainCreator struct {
	iptables *IPTablesController
}

func NewInstanceChainCreator(iptables *IPTablesController) *InstanceChainCreator {
	return &InstanceChainCreator{
		iptables: iptables,
	}
}

func (cc *InstanceChainCreator) Create(logger lager.Logger, handle, instanceId, bridgeName string, ip net.IP, network *net.IPNet) error {
	instanceChain := cc.iptables.InstanceChain(instanceId)

	logger.Info("creating-nat.started")
	if err := cc.iptables.CreateChain("nat", instanceChain); err != nil {
		return err
	}
	logger.Info("creating-nat.finished")

	// Bind nat instance chain to nat prerouting chain
	logger.Info("binding-instance-chain-to-nat.started")
	cmd := exec.Command(cc.iptables.iptablesBinPath, "--wait", "--table", "nat", "-A", cc.iptables.preroutingChain, "--jump", instanceChain, "-m", "comment", "--comment", handle)
	if err := cc.iptables.run("create-instance-chains", cmd); err != nil {
		return err
	}
	logger.Info("binding-instance-chain-to-nat.finished")

	// Enable NAT for traffic coming from containers
	logger.Info("enabling-nat.started")
	cmd = exec.Command("sh", "-c", fmt.Sprintf(
		`(%s --wait --table nat -S %s | grep "\-j MASQUERADE\b" | grep -q -F -- "-s %s") || %s --wait --table nat -A %s --source %s ! --destination %s --jump MASQUERADE -m comment --comment %s`,
		cc.iptables.iptablesBinPath, cc.iptables.postroutingChain, network.String(), cc.iptables.iptablesBinPath, cc.iptables.postroutingChain,
		network.String(), network.String(), handle,
	))
	if err := cc.iptables.run("create-instance-chains", cmd); err != nil {
		return err
	}
	logger.Info("enabling-nat.finished")

	// Create filter instance chain
	logger.Info("creating-filter-chain.started")
	if err := cc.iptables.CreateChain("filter", instanceChain); err != nil {
		return err
	}
	logger.Info("creating-filter-chain.finished")

	// Allow intra-subnet traffic (Linux ethernet bridging goes through ip stack)
	logger.Info("allow-intra-subnet.started")
	cmd = exec.Command(cc.iptables.iptablesBinPath, "--wait", "-A", instanceChain, "-s", network.String(), "-d", network.String(), "-j", "ACCEPT", "-m", "comment", "--comment", handle)
	if err := cc.iptables.run("create-instance-chains", cmd); err != nil {
		return err
	}
	logger.Info("allow-intra-subnet.finished")

	// Otherwise, use the default filter chain
	logger.Info("setup-fallthrough-to-default-filter.started")
	cmd = exec.Command(cc.iptables.iptablesBinPath, "--wait", "-A", instanceChain, "--goto", cc.iptables.defaultChain, "-m", "comment", "--comment", handle)
	if err := cc.iptables.run("create-instance-chains", cmd); err != nil {
		return err
	}
	logger.Info("setup-fallthrough-to-default-filter.finished")

	// Bind filter instance chain to filter forward chain
	logger.Info("bind-instace-to-forward.started")
	cmd = exec.Command(cc.iptables.iptablesBinPath, "--wait", "-I", cc.iptables.forwardChain, "2", "--in-interface", bridgeName, "--source", ip.String(), "--goto", instanceChain, "-m", "comment", "--comment", handle)
	if err := cc.iptables.run("create-instance-chains", cmd); err != nil {
		return err
	}
	logger.Info("bind-instace-to-forward.finished")

	// Create Logging Chain
	return cc.createLoggingChain(logger, handle, instanceId)
}

func (cc *InstanceChainCreator) createLoggingChain(logger lager.Logger, handle, instanceId string) error {
	instanceChain := cc.iptables.InstanceChain(instanceId)
	loggingChain := fmt.Sprintf("%s-log", instanceChain)

	logger.Info("creating-filter-logging-chain.started")
	if err := cc.iptables.CreateChain("filter", loggingChain); err != nil {
		return err
	}
	logger.Info("creating-filter-logging-chain.finished")

	logPrefix := handle
	if len(logPrefix) > 28 {
		logPrefix = logPrefix[0:28]
	}
	logPrefix = logPrefix + " "

	logger.Info("adding-log-rule.started")
	cmd := exec.Command(cc.iptables.iptablesBinPath, "--wait", "-A", loggingChain, "-m", "conntrack", "--ctstate", "NEW,UNTRACKED,INVALID", "--protocol", "all", "--jump", "LOG", "--log-prefix", logPrefix, "-m", "comment", "--comment", handle)
	if err := cc.iptables.run("create-instance-chains", cmd); err != nil {
		return err
	}
	logger.Info("adding-log-rule.finished")

	logger.Info("adding-return.started")
	cmd = exec.Command(cc.iptables.iptablesBinPath, "--wait", "-A", loggingChain, "--jump", "RETURN", "-m", "comment", "--comment", handle)
	if err := cc.iptables.run("create-instance-chains", cmd); err != nil {
		return err
	}
	logger.Info("adding-return.finished")

	return nil
}

func (cc *InstanceChainCreator) Destroy(logger lager.Logger, instanceId string) error {
	instanceChain := cc.iptables.InstanceChain(instanceId)

	// Prune nat prerouting chain
	cmd := exec.Command("sh", "-c", fmt.Sprintf(
		`%s --wait --table nat -S %s 2> /dev/null | grep "\-j %s\b" | sed -e "s/-A/-D/" | xargs --no-run-if-empty --max-lines=1 %s --wait --table nat`,
		cc.iptables.iptablesBinPath, cc.iptables.preroutingChain, instanceChain, cc.iptables.iptablesBinPath,
	))
	if err := cc.iptables.run("prune-prerouting-chain", cmd); err != nil {
		return err
	}

	// Flush instance chain
	if err := cc.iptables.FlushChain("nat", instanceChain); err != nil {
		return err
	}

	// Delete nat instance chain
	if err := cc.iptables.DeleteChain("nat", instanceChain); err != nil {
		return err
	}

	// Prune forward chain
	cmd = exec.Command("sh", "-c", fmt.Sprintf(
		`%s --wait -S %s 2> /dev/null | grep "\-g %s\b" | sed -e "s/-A/-D/" | xargs --no-run-if-empty --max-lines=1 %s --wait`,
		cc.iptables.iptablesBinPath, cc.iptables.forwardChain, instanceChain, cc.iptables.iptablesBinPath,
	))
	if err := cc.iptables.run("prune-forward-chain", cmd); err != nil {
		return err
	}

	// Flush instance chain
	cc.iptables.FlushChain("filter", instanceChain)

	// delete instance chain
	cc.iptables.DeleteChain("filter", instanceChain)

	// delete the logging chain
	instanceLoggingChain := fmt.Sprintf("%s-log", instanceChain)
	cc.iptables.FlushChain("filter", instanceLoggingChain)
	cc.iptables.DeleteChain("filter", instanceLoggingChain)

	return nil
}
