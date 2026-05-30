import 'dart:convert';

import 'package:dio/dio.dart';

ResponseBody responseBodyFromJson(String json, {int statusCode = 200}) {
  final bytes = utf8.encode(json);
  return ResponseBody.fromBytes(
    bytes,
    statusCode,
    headers: {
      'content-type': ['application/json; charset=utf-8'],
    },
  );
}

class MockHttpAdapter implements HttpClientAdapter {
  final ResponseBody Function(RequestOptions options) handler;
  MockHttpAdapter(this.handler);

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<List<int>>? requestStream,
    Future<void>? cancelFuture,
  ) async {
    return handler(options);
  }

  @override
  void close({bool force = false}) {}
}
