// +build !js

package webrtc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carrotsong/ice/v2"
	"github.com/carrotsong/logging"
	"github.com/carrotsong/webrtc/v3/internal/mux"
)

// ICETransport allows an application access to information about the ICE
// transport over which packets are sent and received.
type ICETransport struct {
	lock sync.RWMutex

	role ICERole
	// Component ICEComponent
	// State ICETransportState
	// gatheringState ICEGathererState

	onConnectionStateChangeHandler       atomic.Value // func(ICETransportState)
	onSelectedCandidatePairChangeHandler atomic.Value // func(*ICECandidatePair)

	state ICETransportState

	gatherer *ICEGatherer
	conn     *ice.Conn
	mux      *mux.Mux

	loggerFactory logging.LoggerFactory

	log logging.LeveledLogger
}

// func (t *ICETransport) GetLocalCandidates() []ICECandidate {
//
// }
//
// func (t *ICETransport) GetRemoteCandidates() []ICECandidate {
//
// }
//
// func (t *ICETransport) GetSelectedCandidatePair() ICECandidatePair {
//
// }
//
// func (t *ICETransport) GetLocalParameters() ICEParameters {
//
// }
//
// func (t *ICETransport) GetRemoteParameters() ICEParameters {
//
// }

// NewICETransport creates a new NewICETransport.
func NewICETransport(gatherer *ICEGatherer, loggerFactory logging.LoggerFactory) *ICETransport {
	return &ICETransport{
		gatherer:      gatherer,
		loggerFactory: loggerFactory,
		log:           loggerFactory.NewLogger("ortc"),
		state:         ICETransportStateNew,
	}
}

// Start incoming connectivity checks based on its configured role.
func (t *ICETransport) Start(gatherer *ICEGatherer, params ICEParameters, role *ICERole) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	if gatherer != nil {
		t.gatherer = gatherer
	}

	if err := t.ensureGatherer(); err != nil {
		return err
	}

	agent := t.gatherer.getAgent()
	if agent == nil {
		return fmt.Errorf("%w: unable to start ICETransport", errICEAgentNotExist)
	}

	if err := agent.OnConnectionStateChange(func(iceState ice.ConnectionState) {
		state := newICETransportStateFromICE(iceState)
		t.lock.Lock()
		t.state = state
		t.lock.Unlock()

		t.onConnectionStateChange(state)
	}); err != nil {
		return err
	}
	if err := agent.OnSelectedCandidatePairChange(func(local, remote ice.Candidate) {
		candidates, err := newICECandidatesFromICE([]ice.Candidate{local, remote})
		if err != nil {
			t.log.Warnf("%w: %s", errICECandiatesCoversionFailed, err)
			return
		}
		t.onSelectedCandidatePairChange(NewICECandidatePair(&candidates[0], &candidates[1]))
	}); err != nil {
		return err
	}

	if role == nil {
		controlled := ICERoleControlled
		role = &controlled
	}
	t.role = *role

	// Drop the lock here to allow ICE candidates to be
	// added so that the agent can complete a connection
	t.lock.Unlock()

	var iceConn *ice.Conn
	var err error
	switch *role {
	case ICERoleControlling:
		iceConn, err = agent.Dial(context.TODO(),
			params.UsernameFragment,
			params.Password)

	case ICERoleControlled:
		iceConn, err = agent.Accept(context.TODO(),
			params.UsernameFragment,
			params.Password)

	default:
		err = errICERoleUnknown
	}

	// Reacquire the lock to set the connection/mux
	t.lock.Lock()
	if err != nil {
		return err
	}

	t.conn = iceConn

	config := mux.Config{
		Conn:          t.conn,
		BufferSize:    receiveMTU,
		LoggerFactory: t.loggerFactory,
	}
	t.mux = mux.NewMux(config)

	return nil
}

// restart is not exposed currently because ORTC has users create a whole new ICETransport
// so for now lets keep it private so we don't cause ORTC users to depend on non-standard APIs
func (t *ICETransport) restart() error {
	t.lock.Lock()
	defer t.lock.Unlock()

	agent := t.gatherer.getAgent()
	if agent == nil {
		return fmt.Errorf("%w: unable to restart ICETransport", errICEAgentNotExist)
	}

	if err := agent.Restart(t.gatherer.api.settingEngine.candidates.UsernameFragment, t.gatherer.api.settingEngine.candidates.Password); err != nil {
		return err
	}
	return t.gatherer.Gather()
}

// Stop irreversibly stops the ICETransport.
func (t *ICETransport) Stop() error {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.mux != nil {
		return t.mux.Close()
	} else if t.gatherer != nil {
		return t.gatherer.Close()
	}
	return nil
}

// OnSelectedCandidatePairChange sets a handler that is invoked when a new
// ICE candidate pair is selected
func (t *ICETransport) OnSelectedCandidatePairChange(f func(*ICECandidatePair)) {
	t.onSelectedCandidatePairChangeHandler.Store(f)
}

func (t *ICETransport) onSelectedCandidatePairChange(pair *ICECandidatePair) {
	handler := t.onSelectedCandidatePairChangeHandler.Load()
	if handler != nil {
		handler.(func(*ICECandidatePair))(pair)
	}
}

// OnConnectionStateChange sets a handler that is fired when the ICE
// connection state changes.
func (t *ICETransport) OnConnectionStateChange(f func(ICETransportState)) {
	t.onConnectionStateChangeHandler.Store(f)
}

func (t *ICETransport) onConnectionStateChange(state ICETransportState) {
	handler := t.onConnectionStateChangeHandler.Load()
	if handler != nil {
		handler.(func(ICETransportState))(state)
	}
}

// Role indicates the current role of the ICE transport.
func (t *ICETransport) Role() ICERole {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.role
}

// SetRemoteCandidates sets the sequence of candidates associated with the remote ICETransport.
func (t *ICETransport) SetRemoteCandidates(remoteCandidates []ICECandidate) error {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if err := t.ensureGatherer(); err != nil {
		return err
	}

	agent := t.gatherer.getAgent()
	if agent == nil {
		return fmt.Errorf("%w: unable to set remote candidates", errICEAgentNotExist)
	}

	for _, c := range remoteCandidates {
		i, err := c.toICE()
		if err != nil {
			return err
		}
		err = agent.AddRemoteCandidate(i)
		if err != nil {
			return err
		}
	}

	return nil
}

// AddRemoteCandidate adds a candidate associated with the remote ICETransport.
func (t *ICETransport) AddRemoteCandidate(remoteCandidate ICECandidate) error {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if err := t.ensureGatherer(); err != nil {
		return err
	}

	c, err := remoteCandidate.toICE()
	if err != nil {
		return err
	}

	agent := t.gatherer.getAgent()
	if agent == nil {
		return fmt.Errorf("%w: unable to add remote candidates", errICEAgentNotExist)
	}

	err = agent.AddRemoteCandidate(c)
	if err != nil {
		return err
	}

	return nil
}

// State returns the current ice transport state.
func (t *ICETransport) State() ICETransportState {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.state
}

// NewEndpoint registers a new endpoint on the underlying mux.
func (t *ICETransport) NewEndpoint(f mux.MatchFunc) *mux.Endpoint {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.mux.NewEndpoint(f)
}

func (t *ICETransport) ensureGatherer() error {
	if t.gatherer == nil {
		return errICEGathererNotStarted
	} else if t.gatherer.getAgent() == nil {
		if err := t.gatherer.createAgent(); err != nil {
			return err
		}
	}

	return nil
}

func (t *ICETransport) collectStats(collector *statsReportCollector) {
	t.lock.Lock()
	conn := t.conn
	t.lock.Unlock()

	collector.Collecting()

	stats := TransportStats{
		Timestamp: statsTimestampFrom(time.Now()),
		Type:      StatsTypeTransport,
		ID:        "iceTransport",
	}

	if conn != nil {
		stats.BytesSent = conn.BytesSent()
		stats.BytesReceived = conn.BytesReceived()
	}

	collector.Collect(stats.ID, stats)
}

func (t *ICETransport) haveRemoteCredentialsChange(newUfrag, newPwd string) bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	agent := t.gatherer.getAgent()
	if agent == nil {
		return false
	}

	uFrag, uPwd, err := agent.GetRemoteUserCredentials()
	if err != nil {
		return false
	}

	return uFrag != newUfrag || uPwd != newPwd
}

func (t *ICETransport) setRemoteCredentials(newUfrag, newPwd string) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	agent := t.gatherer.getAgent()
	if agent == nil {
		return fmt.Errorf("%w: unable to SetRemoteCredentials", errICEAgentNotExist)
	}

	return agent.SetRemoteCredentials(newUfrag, newPwd)
}
