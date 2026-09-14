// GameService's Nakama bridge. Nakama executes this file in its sandboxed
// JavaScript runtime; it deliberately has no filesystem, SQL, or secret access.
function gameserviceHealth(ctx, logger, nk, payload) {
  var response = nk.httpRequest(
    "http://control-api:8080/health/live",
    "get",
    { "Accept": "application/json" },
    "",
    2000
  );
  if (!response || response.code !== 200) {
    throw { message: "GameService control plane is unavailable", code: 14 };
  }
  return JSON.stringify({
    nakama: true,
    controlPlane: true,
    userId: ctx.userId || ""
  });
}

function profileString(value, field) {
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value !== "string" || value.length > 128) {
    throw { message: field + " must be a string of at most 128 characters", code: 3 };
  }
  return value;
}

function gameserviceProfile(ctx, logger, nk, payload) {
  if (!ctx.userId) {
    throw { message: "profile RPC requires an authenticated user", code:  unauthenticatedCode() };
  }
  var request = {};
  if (payload) {
    try {
      request = JSON.parse(payload);
    } catch (error) {
      throw { message: "profile payload must be valid JSON", code: 3 };
    }
  }
  if (request.operation && request.operation !== "get" && request.operation !== "patch_public_fields") {
    throw { message: "unsupported profile operation", code: 3 };
  }
  if (request.operation === "patch_public_fields") {
    var fields = request.fields || {};
    if (typeof fields !== "object" || Array.isArray(fields)) {
      throw { message: "profile fields must be an object", code: 3 };
    }
    var allowedFields = { username: true, displayName: true, langTag: true, avatarUrl: true };
    Object.keys(fields).forEach(function (field) {
      if (!allowedFields[field]) {
        throw { message: "unsupported profile field", code: 3 };
      }
    });
    nk.accountUpdateId(
      ctx.userId,
      profileString(fields.username, "username"),
      profileString(fields.displayName, "displayName"),
      null,
      null,
      profileString(fields.langTag, "langTag"),
      profileString(fields.avatarUrl, "avatarUrl"),
      {}
    );
  }
  var account = nk.accountGetId(ctx.userId);
  return JSON.stringify({
    userId: ctx.userId,
    username: account.user.username || "",
    displayName: account.user.displayName || "",
    langTag: account.user.langTag || "",
    avatarUrl: account.user.avatarUrl || "",
    metadata: account.user.metadata || {}
  });
}

function unauthenticatedCode() {
  return 16;
}

function socialString(value, field, required) {
  if (value === null || value === undefined || value === "") {
    if (required) {
      throw { message: field + " is required", code: 3 };
    }
    return "";
  }
  if (typeof value !== "string" || value.length > 256) {
    throw { message: field + " must be a string of at most 256 characters", code: 3 };
  }
  return value;
}

function socialLimit(value) {
  if (value === undefined || value === null) {
    return 100;
  }
  if (typeof value !== "number" || value < 1 || value > 100 || Math.floor(value) !== value) {
    throw { message: "limit must be an integer from 1 to 100", code: 3 };
  }
  return value;
}

function socialState(value) {
  if (value === undefined || value === null) {
    return undefined;
  }
  if (typeof value !== "number" || value < 0 || value > 4 || Math.floor(value) !== value) {
    throw { message: "state must be an integer from 0 to 4", code: 3 };
  }
  return value;
}

function gameserviceSocial(ctx, logger, nk, payload) {
  if (!ctx.userId) {
    throw { message: "social RPC requires an authenticated user", code: unauthenticatedCode() };
  }
  var request = {};
  try {
    request = payload ? JSON.parse(payload) : {};
  } catch (error) {
    throw { message: "social payload must be valid JSON", code: 3 };
  }
  var operation = socialString(request.operation, "operation", true);
  var userAccount = nk.accountGetId(ctx.userId);
  var username = userAccount.user.username || "";
  var target = socialString(request.userId, "userId", false);
  if (target === ctx.userId && (operation === "friends.add" || operation === "friends.delete")) {
    throw { message: "a user cannot target themselves", code: 3 };
  }
  var result;
  if (operation === "friends.list") {
    result = nk.friendsList(ctx.userId, socialLimit(request.limit), socialState(request.state), socialString(request.cursor, "cursor", false));
  } else if (operation === "friends.add") {
    target = socialString(request.userId, "userId", true);
    result = nk.friendsAdd(ctx.userId, username, [target], [socialString(request.username, "username", true)]);
    result = { added: true, userId: target };
  } else if (operation === "friends.delete") {
    target = socialString(request.userId, "userId", true);
    nk.friendsDelete(ctx.userId, username, [target], [socialString(request.username, "username", true)]);
    result = { deleted: true, userId: target };
  } else if (operation === "group.create") {
    var name = socialString(request.name, "name", true);
    var maxCount = request.maxCount === undefined ? 100 : request.maxCount;
    if (typeof maxCount !== "number" || maxCount < 1 || maxCount > 1000 || Math.floor(maxCount) !== maxCount) {
      throw { message: "maxCount must be an integer from 1 to 1000", code: 3 };
    }
    if (request.metadata !== undefined && (typeof request.metadata !== "object" || Array.isArray(request.metadata) || JSON.stringify(request.metadata).length > 2048)) {
      throw { message: "metadata must be an object of at most 2048 characters", code: 3 };
    }
    result = nk.groupCreate(ctx.userId, name, ctx.userId, socialString(request.langTag, "langTag", false), socialString(request.description, "description", false), socialString(request.avatarUrl, "avatarUrl", false), request.open === true, request.metadata || {}, maxCount);
  } else if (operation === "group.join") {
    var groupId = socialString(request.groupId, "groupId", true);
    nk.groupUserJoin(groupId, ctx.userId, username);
    result = { joined: true, groupId: groupId };
  } else if (operation === "group.leave") {
    var leaveGroupId = socialString(request.groupId, "groupId", true);
    nk.groupUserLeave(leaveGroupId, ctx.userId, username);
    result = { left: true, groupId: leaveGroupId };
  } else if (operation === "group.users") {
    result = nk.groupUsersList(socialString(request.groupId, "groupId", true), socialLimit(request.limit), socialState(request.state), socialString(request.cursor, "cursor", false));
  } else if (operation === "groups.mine") {
    result = nk.userGroupsList(ctx.userId, socialLimit(request.limit), socialState(request.state), socialString(request.cursor, "cursor", false));
  } else if (operation === "notifications.list") {
    result = nk.notificationsList(ctx.userId, socialLimit(request.limit), socialString(request.cursor, "cursor", false));
  } else if (operation === "chat.send") {
    var channelId = socialString(request.channelId, "channelId", true);
    var content = request.content;
    if (!content || typeof content !== "object" || Array.isArray(content) || JSON.stringify(content).length > 4096) {
      throw { message: "content must be an object", code: 3 };
    }
    result = nk.channelMessageSend(channelId, content, ctx.userId, username, request.persist !== false);
  } else {
    throw { message: "unsupported social operation", code: 3 };
  }
  return JSON.stringify({ operation: operation, result: result || {} });
}

function tournamentRequiredString(value, field, maxLength) {
  if (typeof value !== "string" || value.length === 0 || value.length > maxLength) {
    throw { message: field + " must be a non-empty string of at most " + maxLength + " characters", code: 3 };
  }
  return value;
}

function gameserviceTournamentRecord(ctx, logger, nk, payload) {
  // This RPC is intentionally server-to-server only. Nakama's runtime is the
  // supported authority for creating tournaments and writing authoritative
  // tournament records; no client session may call it directly.
  if (ctx.userId) {
    throw { message: "tournament record RPC is server-to-server only", code: 7 };
  }
  var request = {};
  try {
    request = payload ? JSON.parse(payload) : {};
  } catch (error) {
    throw { message: "tournament payload must be valid JSON", code: 3 };
  }
  var tournamentId = tournamentRequiredString(request.tournamentId, "tournamentId", 128);
  var ownerId = tournamentRequiredString(request.ownerId, "ownerId", 128);
  var username = request.username || "";
  if (typeof username !== "string" || username.length > 128) {
    throw { message: "username must be a string of at most 128 characters", code: 3 };
  }
  if (typeof request.score !== "number" || !isFinite(request.score) || Math.floor(request.score) !== request.score) {
    throw { message: "score must be an integer", code: 3 };
  }
  var subscore = request.subscore === undefined ? 0 : request.subscore;
  if (typeof subscore !== "number" || !isFinite(subscore) || Math.floor(subscore) !== subscore) {
    throw { message: "subscore must be an integer", code: 3 };
  }
  var duration = request.durationSeconds;
  if (typeof duration !== "number" || duration < 1 || duration > 31536000 || Math.floor(duration) !== duration) {
    throw { message: "durationSeconds must be an integer from 1 to 31536000", code: 3 };
  }
  var resetSchedule = request.resetSchedule || "";
  if (typeof resetSchedule !== "string" || resetSchedule.length > 128) {
    throw { message: "resetSchedule must be a string of at most 128 characters", code: 3 };
  }
  var metadata = request.metadata || {};
  if (typeof metadata !== "object" || Array.isArray(metadata) || JSON.stringify(metadata).length > 2048) {
    throw { message: "metadata must be an object of at most 2048 characters", code: 3 };
  }
  var existing = nk.tournamentsGetId([tournamentId]);
  if (!existing || existing.length === 0) {
    var attempts = request.maxScoreAttempts || 1000000;
    if (typeof attempts !== "number" || attempts < 1 || attempts > 1000000 || Math.floor(attempts) !== attempts) {
      throw { message: "maxScoreAttempts must be an integer from 1 to 1000000", code: 3 };
    }
    nk.tournamentCreate(tournamentId, true, "desc", "best", duration, resetSchedule, metadata, tournamentId, "", 0, 0, 0, 0, attempts, request.joinRequired === true, true);
  }
  if (request.joinRequired === true) {
    nk.tournamentJoin(tournamentId, ownerId, username);
  }
  // Nakama 3.40's JavaScript binding accepts tournament metadata as the JSON
  // string used by its underlying API (the published TypeScript signature is
  // broader than the runtime's actual coercion rule).
  var record = nk.tournamentRecordWrite(tournamentId, ownerId, username, request.score, subscore, JSON.stringify(metadata));
  return JSON.stringify({ tournamentId: tournamentId, ownerId: ownerId, record: record || {} });
}

function InitModule(ctx, logger, nk, initializer) {
  initializer.registerRpc("gameservice.health", gameserviceHealth);
  initializer.registerRpc("gameservice.profile", gameserviceProfile);
  initializer.registerRpc("gameservice.social", gameserviceSocial);
  initializer.registerRpc("gameservice.tournament_record", gameserviceTournamentRecord);
  logger.info("GameService Nakama bridge loaded.");
}
