package srcds

import (
	"net"
	"context"
	"srcds_proxy/proxy/config"
	"time"
	"log"
)

func Dial(addr string) (*net.UDPConn, error) {
	// Dial will create an UDP connection.
	laddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	connection, err := net.DialUDP("udp", nil, laddr)
	if err != nil {
		return nil, err
	}

	if err = setSRCSConnectionOptions(connection); err != nil {
		return nil, err
	}

	return connection, nil
}

func Listen(addr string) (*net.UDPConn, error) {
	// Listen will create a listening UDP connection.
	laddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	connection, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, err
	}

	if err = setSRCSConnectionOptions(connection); err != nil {
		return nil, err
	}

	return connection, nil
}

func Serve(done <-chan struct{}, connection net.UDPConn, handler Handler, timeout time.Duration) error {
	// Serve will read data from a the connection to a buffer and call the handler provided.
	var (
		n          int
		sourceAddr *net.UDPAddr
		err        error
		buf        = make([]byte, MaxDatagramSize)
		timer      *time.Timer // destruction timer, when it triggers, stop the Serve function.
	)

	if timeout > 0 {
		timer = time.NewTimer(timeout)
		go func() {
			<-timer.C
			connection.Close()
		}()
	}

	for {
		// Read into buffer.
		n, sourceAddr, err = connection.ReadFromUDP(buf)

		// When a done event is emitted, exit without handling the message.
		// When the done event is emitted, the connection is also terminated. Thus ReadFromUDP immediately stop with an
		// error but before actually checking the error, we check that the task is not done. So basically, when there is
		// a "done" event, Serve immediately stops.
		select {
		case <-done:
			return nil
		default:
		}

		// If there is a valid message received, reset the destruction timer. If the timer has expired, do not handle
		// the message and return.
		if timeout > 0 && !timer.Reset(timeout) {
			log.Print("DEBUG: Stop Serve after timeout.")
			return nil
		}

		// Check for Read error.
		if err != nil {
			return err
		}

		if err := doHandle(done, buf[:n], connection, sourceAddr, handler); err != nil {
			return err
		}
	}
}

func doHandle(done <-chan struct{}, buf []byte, connection net.UDPConn, sourceAddr *net.UDPAddr, handler Handler) error {
	if len(buf) == 0 {
		return nil
	}

	// If done event is sent, cancel all requests processing.
	ctx, cancelDone := context.WithCancel(context.Background())
	go func() {
		<-done
		cancelDone()
	}()

	// Handler has limited time to process the message.
	ctx, cancelTimeout := context.WithTimeout(ctx, config.HandleTimeout)
	defer cancelTimeout()

	msg := BytesToMessage(buf)
	responseWriter := NewConnectionWriter(connection, sourceAddr)
	return handler.Handle(ctx, responseWriter, msg, UDPAddrToAddressPort(*sourceAddr))
}

func setSRCSConnectionOptions(connection *net.UDPConn) error {
	// Set the buffers size
	if err := connection.SetWriteBuffer(MaxDatagramSize); err != nil {
		return err
	}
	if err := connection.SetReadBuffer(MaxDatagramSize); err != nil {
		return err
	}
	return nil
}
