# Testing quic-go v0.43.1 Behavior

## Key Question

When v0.43.1 successfully connects, **does tcpdump on the server see the packets?**

## Test Steps

1. **On server**, start tcpdump:
   ```bash
   sudo tcpdump -i any -n -v "udp port 9898"
   ```

2. **On client**, test with v0.43.1:
   - Use your actual application with v0.43.1
   - Or create a test program with v0.43.1

3. **Check tcpdump output**:
   - Do packets appear?
   - What do they look like?
   - What's the source/destination?

## Possible Explanations

If v0.43.1 packets **ARE visible** in tcpdump:
- v0.43.1 uses different socket options that work
- v0.43.1 packet structure is different
- v0.43.1 uses different connection method

If v0.43.1 packets **ARE NOT visible** in tcpdump but it still works:
- v0.43.1 might be using a different protocol/encapsulation
- v0.43.1 might be using a different port
- There might be some other mechanism at play

## What to Check

1. **Socket options**: Compare socket options between versions
2. **Packet structure**: Compare packet headers/structure
3. **Connection method**: v0.43.1 might use `Dial()` vs `DialAddr()`
4. **QUIC version**: Different QUIC versions might behave differently

