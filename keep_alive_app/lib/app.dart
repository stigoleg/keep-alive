import 'package:flutter/material.dart';

class KeepAliveApp extends StatelessWidget {
  const KeepAliveApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'KeepAlive',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.blueGrey),
      ),
      home: const Scaffold(
        body: Center(),
      ),
    );
  }
}
