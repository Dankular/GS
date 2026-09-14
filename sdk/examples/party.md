# Nakama party and GameService match flow

Parties belong to Nakama and are created through the authenticated Nakama
real-time socket. GameService does not copy party membership into its own
tables. After the party is formed, the party leader uses Nakama's native
party-matchmaker operation; the resulting match is then represented by the
GameService ticket and match APIs.

The client sequence is:

1. Authenticate both players with Nakama.
2. The leader calls `createParty(open, maxPlayers)`.
3. Invited players call `joinParty(partyId)`; closed-party requests are
   accepted by the leader.
4. The leader calls `partyMatchmakerAdd` with the mode/build/region query.
5. The GameService allocation, join-claim, result, reward, and leaderboard
   paths remain authoritative after the matchmaker callback.
6. The client calls `leaveParty(partyId)` when the session ends.

The Nakama SDK version must match the pinned Nakama server image. Party
membership is intentionally not accepted as a client-supplied authority in a
GameService command.
