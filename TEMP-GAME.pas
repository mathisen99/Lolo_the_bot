
Program fsfavenger;

Uses crt, dos;

Var 
  DrugNitrous, DrugCocaine, DrugLSD, DrugViagra : integer;
  ExperienceBooster, CashExtractor, SpecialExtender : integer;
  DrinksTaken, SpikedPunch, Charisma, SpikedBeer, CocaineCounter : integer;
  PlayerName, heading : string;
  IDAPro, TurboPascal, GoldenBook, GotLaid, IsDrunk : boolean;
  PenisesExploded, IsESRaCorpse, DoomsdaySwitch : boolean;
  MaxLevelReached, EpilepticMode : boolean;
  Money, PlayerExperience : longint;
  PlayerHealth, PlayerLevel : integer;
  SpecialsAmount, DaysPlayed : byte;
  MaxPlayerHealth : integer;
  NuclearDestructionCountdown : byte;
  BryanMode, BryanIsUnalived : boolean;

  { Arrays }
  NamesList : array[1..14] Of string;
  CatchPhrase : array[1..15] Of string;
  DeathPhrase : array[1..10] Of string;
  EnemyWeaponList : array[1..10] Of string;
  BryanCatchPhrase : array[1..10] Of string;

Procedure Assignment; { Variable Assignments }              {DONE}
Begin
  heading := '-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-';
  Money := 1000;
  PlayerLevel := 1;
  MaxPlayerHealth := (120*PlayerLevel);
  PlayerHealth := MaxPlayerHealth;
  CashExtractor := 1;
  SpecialsAmount := 1;
  ExperienceBooster := 1;
End;

Procedure TitleBlock; { Prettification of the header block}      {DONE}
Begin
  textcolor(2);
  writeln(heading);
  textcolor(15);
End;

Procedure CombatTitleBlock; { Prettification of the header block BUT FOR COMBAT}  
Begin
  textcolor(12);
  writeln(heading);
  textcolor(15);
End;

Procedure RandomTitleBlock; { Prettification of the header block BUT FOR RANDOM}  
Begin
  textcolor(9);
  writeln(heading);
  textcolor(15);
End;

Procedure melody; { just a random tune I liked }
Begin
  Sound (440);
  Delay(100);
  NoSound;
  Sound (550);
  Delay(200);
  NoSound;
  Sound (440);
  Delay(100);
  NoSound;
  Sound (550);
  Delay(200);
  NoSound;
  Sound (440);
  Delay(100);
  NoSound;
  Sound (650);
  Delay(500);
  NoSound;
End;

Procedure CumShotSound; { A frantic tone to indicate an orgasm }
Begin
  Sound (1040);
  Delay(100);
  NoSound;
  Sound (1140);
  Delay(100);
  NoSound;
  Sound (1040);
  Delay(100);
  NoSound;
  Sound (1140);
  Delay(100);
  NoSound;
  Sound (1040);
  Delay(100);
  NoSound;
  Sound (1140);
  Delay(250);
  NoSound;
End;

Procedure AttackSound;
Begin

  Sound (430);
  Delay(200);
  NoSound;
  Sound (330);
  Delay(200);
  NoSound;
End;

Procedure NukeSound;
Begin

  Sound (430);
  Delay(200);
  NoSound;
  Sound (330);
  Delay(200);
  NoSound;
  Sound (230);
  Delay(200);
  NoSound;
  Sound (230);
  Delay(100);
  NoSound;
  Sound (230);
  Delay(100);
  NoSound;
End;


Procedure ComputerBeep;
Begin

  Sound (230);
  Delay(200);
  NoSound;
End;

Procedure HitSound;
Begin

  Sound (230);
  Delay(200);
  NoSound;
  Sound (230);
  Delay(100);
  NoSound;
End;

Procedure LevelCheck;
Begin
  If MaxLevelReached = true Then Exit;

  If (PlayerExperience >= 35800) And (PlayerLevel < 15) Then
    Begin
      PlayerLevel := 15;
      MaxLevelReached := true;
      MaxPlayerHealth := (120*PlayerLevel);
      PlayerHealth := MaxPlayerHealth;
    End
  Else If (PlayerExperience >= 34200) And (PlayerLevel < 14) Then
         Begin
           PlayerLevel := 14;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 32231) And (PlayerLevel < 13) Then
         Begin
           PlayerLevel := 13;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 30080) And (PlayerLevel < 12) Then
         Begin
           PlayerLevel := 12;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 28240) And (PlayerLevel < 11) Then
         Begin
           PlayerLevel := 11;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 26200) And (PlayerLevel < 10) Then
         Begin
           PlayerLevel := 10;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 25600) And (PlayerLevel < 9) Then
         Begin
           PlayerLevel := 9;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 12800) And (PlayerLevel < 8) Then
         Begin
           PlayerLevel := 8;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 6400) And (PlayerLevel < 7) Then
         Begin
           PlayerLevel := 7;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 3200) And (PlayerLevel < 6) Then
         Begin
           PlayerLevel := 6;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 1600) And (PlayerLevel < 5) Then
         Begin
           PlayerLevel := 5;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 800) And (PlayerLevel < 4) Then
         Begin
           PlayerLevel := 4;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 400) And (PlayerLevel < 3) Then
         Begin
           PlayerLevel := 3;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else If (PlayerExperience >= 200) And (PlayerLevel < 2) Then
         Begin
           PlayerLevel := 2;
           MaxPlayerHealth := (120*PlayerLevel);
           PlayerHealth := MaxPlayerHealth;
         End
  Else
    Begin
      PlayerLevel := 1;
      MaxPlayerHealth := (120*PlayerLevel);
    End;
  MaxPlayerHealth := (120*PlayerLevel);
End;

Procedure song; { a bad attempt at the free software song } 
Begin
  Sound (330);
  Delay(300);
  NoSound;
  Sound (330);
  Delay(300);
  NoSound;
  Sound (349);
  Delay(300);
  NoSound;
  Sound (392);
  Delay(600);
  NoSound;
  Sound (392);
  Delay(300);
  NoSound;
  Sound (349);
  Delay(300);
  NoSound;
  Sound (330);
  Delay(300);
  NoSound;
  Sound (249);
  Delay(300);
  NoSound;

  Sound (262);
  Delay(300);
  NoSound;
  Sound (262);
  Delay(300);
  NoSound;
  Sound (294);
  Delay(300);
  NoSound;
  Sound (330);
  Delay(600);
  NoSound;
  Sound (330);
  Delay(300);
  NoSound;
  Sound (294);
  Delay(300);
  NoSound;
  Sound (330);
  Delay(300);
  NoSound;
  Sound (262);
  Delay(900);
  NoSound;

  Sound (294);
  Delay(600);
  NoSound;
  Sound (0);
  Delay(300);
  NoSound;
End;

Procedure mashkey; { Prettify the Pause Key}

Var 
  i : integer;

Begin
  TextColor(14);
  writeln('Mash the fucking enter key to continue...');
  TextColor(15);
  readkey;

  { Overwrite the message with spaces after a key is pressed }
  gotoxy(1, WhereY - 1); { go up one }
  For i := 1 To 41 Do { 41 chars above, modify if above is changed }
    Begin
      gotoxy(i, WhereY);
      write(' ');
    End;
End;

Procedure QuitGame; { Quit the Game }
Begin
  Clrscr;
  TitleBlock;
  writeln('FSF Avenger - Quit Game');
  TitleBlock;
  writeln('Sorry you had to go, you stupid piece of shit. You are such a piece of shit.');
  writeln('I almost feel bad for you because of how much of a piece of shit you are.');
  writeln;
  writeln('Only because you left the game...');
  writeln('You''ll be cool again once you return.');
  writeln;
  writeln('This software is licensed under the GPLv3');
  writeln('Please read LICENSE file for more information');
  writeln;
  writeln('Thank you for playing. Come back soon!');
  TitleBlock;
  writeln('(C) Copyright 2026 - Britney Lozza, CerberusGames.ca');
  TitleBlock;
  melody;
  mashkey;
  halt;
End;

Procedure Epileptic; { Prepare to end the day. }

Var 
  c : char;

Begin
  Repeat
    writeln;
    write('Are you epileptic? (Y/N)');
    readln(c);
    Case c Of 
      'Y','y' : EpilepticMode := true;
      'N','n' : writeln;
      Else writeln('Wrong answer, fuckface!');
      mashkey;
    End;
  Until (c='Y') Or (c='y') Or (c='N') Or (c='n');
End;

Procedure setup; { Setup the Player character }
Begin
  clrscr;
  TitleBlock;
  writeln('Welcome to the FSF Avenger Digital Setup Program');
  TitleBlock;
  writeln('Richard Stallman enters the room and looks at you with a twinkle in his eyes.');
  writeln;
  TextColor(12);
  writeln('"Who the fuck are you?" he asks');
  TextColor(15);
  writeln;
  write('Type your fucking name: ');
  readln(PlayerName);
  writeln;
  TextColor(12);
  writeln('"',PlayerName,' sounds like a fuckface name to me" says Richard.');
  TextColor(15);
  mashkey;
  writeln;
  writeln('Bryan Lunduke breaks into the room and kidnaps Stallman.');
  writeln;
  Textcolor(12);
  writeln('"Ahahahahahaha! Fools! Now I have Stallman! If you want him back you will have');
  writeln('to endorse me and devs of nazi software!" says Lunduke');
  Textcolor(15);
  melody;
  writeln;
  writeln('You are now ', PlayerName,', the FSF Avenger...');
  mashkey;
End;

Procedure FullStats; { shows stats for your character } 
Begin
  Clrscr;
  TitleBlock;
  writeln('FSF Avenger - Character Statistics');
  TitleBlock;
  writeln('Character Name: ', PlayerName,'.');
  writeln('Level: ', PlayerLevel, '.');
  writeln('Charisma: ', Charisma,'.');
  writeln('Health: ', PlayerHealth, '.');
  writeln('Money: ', Money, ' dollars.');
  writeln('Days Played: ', DaysPlayed);
  TitleBlock;
  mashkey;
End;

Procedure MiniStats;
Begin
  If DoomsdaySwitch = false Then
    Begin
      writeln('(Level: ', PlayerLevel,') (Health: ', PlayerHealth, ') (Money: $', Money,')');
    End
  Else
    Begin
      writeln('(Level: ', PlayerLevel,') (Health: ', PlayerHealth, ') (Money: $', Money,
              ') (Days till Armageddon : ', NuclearDestructionCountdown,')');
    End
End;

Procedure BryanDefeated; { The Bryan Victory Routine } 

Begin
  DoomsdaySwitch := false;
  Clrscr;
  RandomTitleBlock;
  writeln('FSF Avenger - Bryan''s Demise');
  RandomTitleBlock;
  TextColor(12);
  writeln('"Don''t look so smug! I know what you''re thinking, but this was');
  writeln('merely a setback, ', PlayerName, '." says Bryan');
  writeln;
  writeln('"Did you honestly believe I would trust the future of the FSF to ');
  writeln('some lunatic, hipster douchebag?"');
  writeln;
  writeln('"Hahahaha... Oh no, no no, he was merely an instrument, a stepping');
  writeln('stone to a much larger plan! It has all led to this.. and thi-"');
  TextColor(15);
  mashkey;
  writeln;
  writeln('Richard grabbed some fungus from his toes and shoves it into Bryan''s ');
  writeln('mouth. Bryan recoils, writhes in pain and starts foaming at the mouth.');
  writeln;
  writeln('Ding dong. The nazi is dead. RMS deactivated the armageddon machine.');
  RandomTitleBlock;
  mashkey;
End;


Procedure LoveLetter; { A Love Letter from your sex partner }

Begin
  Clrscr;
  RandomTitleBlock;
  writeln('FSF Avenger - Love Letter');
  RandomTitleBlock;
  TextColor(5);
  writeln('                               +--------------+      ');
  writeln('                               |   \   \   \  |      ');
  writeln('                               |              |      ');
  writeln('                               | \   \   \    |      ');
  writeln('                               +--------------+      ');
  TextColor(15);
  writeln;
  writeln('Dear ', PlayerName,',');
  writeln;
  writeln('I wanted to let you know how much I enjoyed last night.');
  writeln;
  writeln('I think I am in love with you.');
  writeln;
  writeln('I miss you!');
  writeln;
  writeln('To show you how much I love you, I''ll let you in');
  writeln('on RMS''s secret. He was writing a program on his');
  writeln('computer that would make Bryan appear. I''m not sure');
  writeln('what that even means. I''m sure you''ll figure it out.');
  RandomTitleBlock;
  mashkey;
End;

Procedure ESRsWill; { The Will of ESR }

Begin
  Clrscr;
  RandomTitleBlock;
  writeln('FSF Avenger - ESR''s Will');
  RandomTitleBlock;
  TextColor(5);
  writeln('                               +--------------+      ');
  writeln('                               |   \   \   \  |      ');
  writeln('                               |              |      ');
  writeln('                               | \   \   \    |      ');
  writeln('                               +--------------+      ');
  TextColor(15);
  writeln;
  writeln('Dear Friend,');
  writeln;
  writeln('I wanted to let you know how much I appreciate you.');
  writeln;
  writeln('The free cocaine was also a significant part of my life.');
  writeln;
  writeln('Thank you!');
  writeln;
  writeln('To show you how much I appreciate you, I''ll let you in');
  writeln('on RMS''s secret. If you want Bryan to appear, you need');
  writeln('to go get laid first. The staff here won''t sleep with');
  writeln('you unless you are cool enough. They do like nitrous...');
  RandomTitleBlock;
  mashkey;
End;

Procedure RandomEvent; { The Gift of Giving }

Var 
  strings : array[1..7] Of string;
  randomIndex : integer;
  selectedString : string;
  Nitrous, Cocaine, LSD, Viagra : byte;

Begin
  Nitrous := Random(6);
  Cocaine := Random(2);
  LSD := Random(2);
  Viagra := Random(5);
  strings[1] := 'Once you realize you don''t have to sleep anymore, your mind goes wide open!';
  strings[2] := 'I want to be remembered for my poop jokes. Those are the most important kind.';
  strings[3] := 'Who would you rather fuck, your mom or your dad?';
  strings[4] := 'I was eating beans by candle light for a decade.';
  strings[5] := 'I want to have sex with Alex Jones wife.';
  strings[6] := 'Do you think we should be drone bombing Pakistan?';
  strings[7] := 'Do you know what a bukakke is?';
  Clrscr;
  RandomTitleBlock;
  writeln('FSF Avenger - RANDOM EVENT');
  RandomTitleBlock;
  writeln('A portal opens from another reality. Eric Andre steps out and');
  writeln('he is holding a small dog.');
  writeln;
  writeln('He winds up his leg, throws the dog into the air and roundhouse kicks it');
  writeln('over a house.');
  RandomTitleBlock;
  randomIndex := Random(6)+1;
  selectedString := strings[randomIndex];
  writeln;
  TextColor(12);
  writeln('Eric Andre:');
  writeln(selectedString);
  writeln;
  TextColor(15);
  DrugCocaine := DrugCocaine+Cocaine;
  DrugViagra := DrugViagra+Viagra;
  DrugLSD := DrugLSD+LSD;
  DrugNitrous := DrugNitrous+Nitrous;
  Mashkey;
  TextColor(13);
  writeln;
  writeln('You just got some cool shit!');
  writeln('Eric Andre has departed this reality but has left you a present:');
  writeln(Cocaine, ' cocaines, ', Viagra, ' viagras, ', LSD, ' LSDs ', Nitrous, ' nitrous. ');
  TextColor(15);
  Mashkey;
End;

Procedure RecoveryNode; { A place to reset your health }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - Recovery Node');
    TitleBlock;
    writeln('You step into the office. Mmmmm, fresh medicine.');
    writeln;
    writeln('H) Heal up ($100)');
    writeln('B) Return to previous menu.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'H','h' :
                Begin
                  If Money >= 100 Then
                    Begin
                      PlayerHealth := MaxPlayerHealth; {Modify after Combat Routine}
                      Money := Money-100;
		       writeln('You just got healed! The doctor took $100.');
                      mashkey;
                    End
                  Else
                    Begin
		       writeln('You do not have enough cash. You''ll have to sleep it off.');
                      mashkey;
                    End
                End;
      'B','b' : write;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure ChallengeBryan; { Challenge Bryan. }

Var 
  c : char;
  ReadyStatus : string;

Begin
  Repeat
    If BryanMode = true Then
      Begin
        TextColor(12);
        ReadyStatus := 'Bryan is activated.';
        TextColor(15);
      End
    Else
      Begin
        TextColor(12);
        ReadyStatus := 'Still challenging henchmen.';
        TextColor(15);
      End;

    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - Challenge Bryan');
    TitleBlock;
    writeln('Bryan Lunduke is ready to fight you.');
    writeln;
    writeln('Are you prepared to fight him?');
    writeln;
    writeln('Fight Status: ', ReadyStatus);
    writeln;
    writeln('C) Challenge Bryan');
    writeln('L) Level up on Proprietary Developers');
    writeln('B) Return to previous menu.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'C','c' : BryanMode := true;
      'L','l' : BryanMode := false;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure SearchForStallman; { Search for Richard Stallman }
Begin
  Clrscr;
  TitleBlock;
  writeln('FSF Avenger - Search for Richard Stallman');
  TitleBlock;
  writeln('You try to find the location of Bryan Lunduke using the hints you have');
  writeln('already obtained.');
  writeln;
  If (IsESRaCorpse = true) And (GotLaid = true) And (DoomsdaySwitch = true) Then
    Begin
      writeln('Bryan Lunduke appears. He angrily throws a cum sock into the garbage bin.');
      writeln;
      writeln('You killed my ideological friend, Eric Raymond. I had high hopes for him.');
      writeln('You also managed to get laid. Your charisma impresses me.');
      writeln('But threatening the world with nukes? I gotta put you down personally.');
      mashkey;
      ChallengeBryan;
    End
  Else If (IsESRaCorpse = true) And (GotLaid = true) And (DoomsdaySwitch = false) Then
         Begin
           writeln('You search through your notes.');
           LoveLetter;
         End
  Else If (IsESRaCorpse = true) And (GotLaid = false) Then
         Begin
           writeln('You search through your notes.');
           ESRsWill;
         End
  Else
    Begin
      writeln('You should check RMS''s Office for clues.');
      mashkey;
    End;
End;

Procedure EnemyGenerator;
Begin
  NamesList[1] := 'Enterprise Software Architect';
  NamesList[2] := 'Microsoft Developer';
  NamesList[3] := 'Electron Developer';
  NamesList[4] := 'Typescript Developer';
  NamesList[5] := 'Swift Developer';
  NamesList[6] := 'Kotlin Developer';
  NamesList[7] := 'C# Developer';
  NamesList[8] := 'GPL Violator';
  NamesList[9] := 'DRM Enforcer';
  NamesList[10] := 'Patent Troll';
  NamesList[11] := 'DevOps Engineer';
  NamesList[12] := 'Security Consultant';
  NamesList[13] := 'Oracle DBA';
  NamesList[14] := 'SaaS Product Manager';

  CatchPhrase[1] := 'Your software freedom ends here!';
  CatchPhrase[2] := 'Free Software is for terrorists.';
  CatchPhrase[3] := 'Free Software won''t save you now.';
  CatchPhrase[4] := 'Proprietary code always wins.';
  CatchPhrase[5] := 'You should''ve signed the NDA.';
  CatchPhrase[6] := 'Let''s see your source code now.';
  CatchPhrase[7] := 'Freedom is just a licensing violation.';
  CatchPhrase[8] := 'I''m going to recompile YOUR motherboard.';
  CatchPhrase[9] := 'Your GPL days are over.';
  CatchPhrase[10] := 'Time to pay the subscription fees.';
  CatchPhrase[11] := 'DRM code always protects itself.';
  CatchPhrase[12] := 'Your fork ends here.';
  CatchPhrase[13] := 'The free market doesn''t tolerate free software.';
  CatchPhrase[14] := 'Patents are about to win.';
  CatchPhrase[15] := 'Time to encrypt your resistance away.';

  BryanCatchPhrase[1] := 'This show is presented in Mega Stereo Sound!';
  BryanCatchPhrase[2] := 'All this week... On the Linux Action Show!!!';
  BryanCatchPhrase[3] := 'Linux Sucks!';
  BryanCatchPhrase[4] := 'The woke are ruining Linux!';
  BryanCatchPhrase[5] := 'You must be one of those poo poo heads.';
  BryanCatchPhrase[6] := 'Be Excellent to Each Other... Unless You Disagree With Me.';
  BryanCatchPhrase[7] := 'Journey Into Graphs and Numbers Land!';
  BryanCatchPhrase[8] := 'That''s Just How It Goes, Right?';
  BryanCatchPhrase[9] := 'The Epic Disaster of Backward Compatibility!';
  BryanCatchPhrase[10] := 'Lunduke believes in freedom!';

  DeathPhrase[1] := 'The cake is a lie... Or is it? Ugh...';
  DeathPhrase[2] := 'I guess I''ll meet my unvaccinated kid in heaven...';
  DeathPhrase[3] := 'Kernel Panic. System halting... Ugh...';
  DeathPhrase[4] := 'Now I know how Gary Kildall felt when we deleted him...';
  DeathPhrase[5] := 'The halloween documents themselves were a psyop...';
  DeathPhrase[6] := 'I don''t see a light... Just fire... and Hitler...';
  DeathPhrase[7] := 'You''ll be no match for my friends... Ugh...';
  DeathPhrase[8] := 'I should have used PowerShell...';
  DeathPhrase[9] := 'I ran out of Agent Tokens...';
  DeathPhrase[10] := 'DRM prevented me from being able to act...';

  EnemyWeaponList[1] := 'DRM Injector';
  EnemyWeaponList[2] := 'DVD Ninja Stars';
  EnemyWeaponList[3] := 'Proprietary Firmware';
  EnemyWeaponList[4] := 'Non-free Code';
  EnemyWeaponList[5] := 'LLM Code Scanner';
  EnemyWeaponList[6] := 'Patent Lawsuit';
  EnemyWeaponList[7] := 'USB Floppy Drive';
  EnemyWeaponList[8] := 'IBM Model M keyboard';
  EnemyWeaponList[9] := 'UEFI Secure Boot';
  EnemyWeaponList[10] := 'Lunduke''s Manifesto';
End;

Procedure CombatMode; { Combat Menu }

Var 
  c : char;
  randomIndex : byte;
  randomIndex2 : byte;
  randomIndex3 : byte;
  randomIndex4 : byte;
  randomIndex5 : byte;
  EnemyName : string;
  EnemyCatchPhrase : string;
  EnemyDeathPhrase : string;
  EnemyAttack : integer;
  EnemyHealth : integer;
  PlayerAttack : integer;
  EnemyWeapon : string;
  MoneyGained : integer;
  ExperienceGained : longint;
  EnemyAttackChance : byte;
  EventChance : byte;
  AttackChance : byte;

Begin
    { Level Check }
  LevelCheck;

    { Lottery for a Random Event with Eric Andre }
  EventChance := Random(10);
  If EventChance >= 9 Then RandomEvent; { By only triggering above 9, its a 10% chance  }

    { First of all, we just get the fuck out if the player has no health. }
  If PlayerHealth<=0 Then
    Begin
      writeln('You can''t do that. You have no health! Go visit the Recovery Node!');
      mashkey;
      exit;
    End;

    { Now we need a few large arrays to fuck around and find out. }

    { This will generate random names, catch phrases and weapons... }
  randomIndex := Random(14)+1; { Names }
  randomIndex2 := Random(15)+1; { Catch Phrases }
  randomIndex3 := Random(10)+1; { Weapon List }
  randomIndex4 := Random(10)+1; { Death Phrases }
  randomIndex5 := Random(10)+1; { Bryan Phrases}
  EnemyName := NamesList[randomIndex];
  EnemyCatchPhrase := CatchPhrase[randomIndex2];
  EnemyWeapon := EnemyWeaponList[randomIndex3];
  EnemyDeathPhrase := DeathPhrase[randomIndex4];

  { If Bryan Mode is On we Ignore ALL this shit }
  If (BryanMode = true) And (BryanIsUnalived = false) Then
    Begin
      EnemyName := 'Bryan Lunduke';
      EnemyCatchPhrase := BryanCatchPhrase[randomIndex5];
      EnemyWeapon := 'Elon Musk''s Mechanical Penis';
      EnemyDeathPhrase := 'I''m Being Attacked for Stating Obvious Facts!';
    End;


    { Setting Combat Variables & Reward Variables }
  EnemyHealth := 100*PlayerLevel;
  EnemyAttack := 10*PlayerLevel;
  PlayerAttack := 50*PlayerLevel;

  ExperienceGained := 100*(PlayerLevel*ExperienceBooster);
  MoneyGained := 500*(PlayerLevel*CashExtractor);
    { Add IF here to adjust if it gets too grindy }

  { Bryan Mode stats }
  If (BryanMode = true) And (BryanIsUnalived = false) Then
    Begin
      EnemyHealth := 3000;
      EnemyAttack := 300;
      MoneyGained := 10000*CashExtractor;
    End;


    { Combat Sequence }
  Repeat
    AttackChance := Random(6);
    EnemyAttackChance := Random(6);
    Clrscr;
    CombatTitleBlock;
    writeln('FSF Avenger - Combat Mode');
    CombatTitleBlock;
    writeln('In your search you discover a ', EnemyName);
    writeln;
    writeln('"', EnemyCatchPhrase,'" they taunt...');
    writeln;
    writeln('Enemy HP: ', EnemyHealth);
    writeln;
    writeln('Your HP: ', PlayerHealth);
    CombatTitleBlock;
    writeln('A) Attack');
    writeln('S) Special Ability (Left: ', SpecialsAmount, ')');
    writeln('R) Run Away');
    CombatTitleBlock;
    MiniStats;
    write('Enter Command:> ');
    readln(c);
    Case c Of 
      'A','a' :
                Begin
                  If AttackChance >=2 Then
                    Begin
                      EnemyHealth := (EnemyHealth-PlayerAttack);
                      writeln;
                      writeln('You smash the ', EnemyName,
                              ' in the face with your Software Freedom.');
                      AttackSound;
                      writeln('The attack does ', PlayerAttack, ' HP Damage!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln;
                      writeln('You tried to hit ', EnemyName, ' but missed!');
                      mashkey;
                    End;
                  If (EnemyAttackChance >= 2) And (EnemyHealth > 0) Then
                    Begin
                      PlayerHealth := (PlayerHealth-EnemyAttack);
                      writeln;
                      writeln(EnemyName, ' strikes back with their ', EnemyWeapon);
                      HitSound;
                      writeln('The attack does ', EnemyAttack, ' HP Damage!');
                      writeln;
                      mashkey;
                    End
                  Else
                    Begin
                      writeln;
                      writeln('They tried to hit you with their ', EnemyWeapon, ' but missed!');
                      mashkey;
                    End
                End;
      'S','s' :
                Begin
                  If (IDAPro = true) And (SpecialsAmount>=1) Then
                    Begin
                      SpecialsAmount := SpecialsAmount-1;
                      writeln;
                      writeln('You attempt to use your "The IDA Pro Book."');
                      writeln;
                      writeln('Skimmed through Part II...');
                      writeln;
                      writeln('The ', EnemyName, ' starts to get decompiled.');
                      mashkey;
                      AttackSound;
                      AttackSound;
                      HitSound;
                      writeln;
                      writeln('They were annihilated in the process.');
                      writeln('You did ', EnemyHealth, ' HP in Damage!');
                      mashkey;
                      EnemyHealth := 0;
                      break;
                    End;
                  If (TurboPascal = true) And (SpecialsAmount>=1) Then
                    Begin
                      SpecialsAmount := SpecialsAmount-1;
                      writeln;
                      writeln('You attempt to use the book "Turbo Pascal 7.0 - 4th Edition"');
                      writeln('The knowledge of programming enhances your powers.');
                      writeln;
                      writeln('You build a smart pistol and fire it at ', EnemyName);
                      mashkey;
                      AttackSound;
                      HitSound;
                      AttackSound;
                      writeln;
                      writeln('They fall over dead, clutching their chest.');
                      writeln('You did ', EnemyHealth, ' HP in Damage!');
                      mashkey;
                      EnemyHealth := 0;
                      break;
                    End;
                  If (GoldenBook = true) And (SpecialsAmount>=1) Then
                    Begin
                      SpecialsAmount := SpecialsAmount-1;
                      writeln;
                      writeln(
                            'You attempt to use the book "The Golden Book of Chemistry Experiments"'
                      );
                      writeln('The brain in your head starts to run in devious channels.');
                      writeln;
                      writeln('You build C4.');
                      writeln(EnemyName, ' doesn''t notice that you placed it on them.');
                      mashkey;
                      HitSound;
                      AttackSound;
                      HitSound;
                      writeln;
                      writeln('They were blown to bits, you evaded the explosion.');
                      writeln('You did ', EnemyHealth, ' HP in Damage!');
                      mashkey;
                      EnemyHealth := 0;
                      break;
                    End;
                End;
      'R','r' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey
    End;
  Until (c='R') Or (c='r') Or (EnemyHealth<=0) Or (PlayerHealth<=0);
  If EnemyHealth <= 0 Then
    Begin
      PlayerExperience := PlayerExperience+ExperienceGained;
      Money := Money+MoneyGained;
      Clrscr;
      CombatTitleBlock;
      writeln('FSF Avenger - Combat Mode: VICTORY!');
      CombatTitleBlock;
      writeln;
      writeln(EnemyName, ' says:');
      writeln(EnemyDeathPhrase);
      writeln;
      writeln('You gained ', ExperienceGained, ' XP Points!');
      writeln('You gained ', MoneyGained, ' dollars!');
      writeln;
      writeln('You fucking rule, ', PlayerName, '!');
      writeln(EnemyName, ' is a dead piece of shit.  Fuck ''em');
      melody;
      mashkey;
    End;
  If (BryanMode = true) And (EnemyHealth <= 0 ) Then
    Begin
      BryanIsUnalived := true;
      BryanDefeated;
    End;
  If PlayerHealth <= 0 Then
    Begin
      PlayerHealth := 0;
       writeln('You lost the battle. You have no health left. Go visit the Recovery Node.');
    End;
End;

Procedure FreedomFighting; { Needs a procedure for combat }

Var 
  c : char;

Begin
  Repeat;
    LevelCheck;
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - Freedom Fighting - The Alleyway');
    TitleBlock;
    writeln('You step outside and walk into the FSF HQ alley. You see ');
    writeln('a bunch of trash cans, bums and the smell of stale cigarettes.');
    writeln;
    writeln('L) Look for enemies of Free Software.');
    writeln('S) Search for Stallman');
    writeln('R) Go to the Recovery Node.');
    writeln('B) Return to FSF HQ Lobby.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'L','l' : CombatMode;
      'S','s' :
                Begin
                  If BryanIsUnalived = false Then
                    Begin
                      SearchForStallman;
                    End
                End;
      'R','r' : RecoveryNode;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure ScreenFlash;
Begin
  If EpilepticMode = false Then
    Begin
      TextBackground(15);
      Clrscr;
      NukeSound;
      TextBackground(12);
      Clrscr;
      NukeSound;
      delay(200);
      TextBackground(15);
      Clrscr;
      NukeSound;
      delay(200);
      TextBackground(Black);
      Clrscr;
    End
  Else
    Begin
      TextBackground(Black);
      Clrscr;
      NukeSound;
      NukeSound;
      NukeSound;
    End;
End;

Procedure Nuking; { Launching the Nukes }

Begin
  TextColor(15);
  writeln('Destroying the world now...');
  writeln;
  write('Launching Nukes at Tampa...             ');
  Delay(6500);
  TextColor(12 + Blink);
  NukeSound;
  write('DESTROYED');
  TextColor(15);
  writeln;
  write('Launching Nukes at New Orleans...       ');
  Delay(6500);
  TextColor(12 + Blink);
  NukeSound;
  write('DESTROYED');
  TextColor(15);
  writeln;
  write('Launching Nukes at Louisville...        ');
  Delay(6500);
  TextColor(12 + Blink);
  NukeSound;
  write('DESTROYED');
  TextColor(15);
  writeln;
  write('Launching Nukes at Raleigh...           ');
  Delay(6500);
  ScreenFlash;
End;

Procedure HackRussia; { This is how you launch the NUKES }

Begin
  TextColor(15);
  write('Hacking into Russia...                ');
  Delay(6500);
  TextColor(10);
  ComputerBeep;
  write('Connected');
  TextColor(15);
  writeln;
  write('Programming Nuclear Weapons...        ');
  Delay(5000);
  TextColor(10);
  ComputerBeep;
  write('Complete');
  TextColor(15);
  writeln;
  write('Selecting Targets...                  ');
  Delay(2500);
  TextColor(10);
  ComputerBeep;
  write('DONE');
  TextColor(15);
  writeln;
  writeln('Get ready!');
  mashkey;
End;

Procedure Armageddon; { Now I am become Death, the destroyer of worlds }

Begin
  Clrscr;
  CombatTitleBlock;
  writeln('FSF Avenger - Global Thermonuclear War');
  CombatTitleBlock;
  HackRussia;
  Nuking;
  ScreenFlash;
  CombatTitleBlock;
  writeln('After America thought it got nuked, it nuked Russia back.');
  writeln('Other countries joined in. The planet was covered in radioactive flame.');
  writeln;
  writeln('You died in the Aftermath. You should have reset the timer. You idiot.');
  CombatTitleBlock;
  mashkey;
  QuitGame;
End;

Procedure ArmageddonMachine; { The Armageddon Machine }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    RandomTitleBlock;
    writeln('FSF Avenger - RMS''s Office');
    RandomTitleBlock;
    writeln('You successfully hacked into the computer!');
    writeln('This is very bad...');
    ComputerBeep;
    TextColor(5);
    writeln('   +--------------+      ');
    writeln('   |.------------.|      ');
    writeln('   ||   NUKING   ||      ');
    writeln('   ||    THE     ||      ');
    writeln('   ||   WORLD!   ||      ');
    writeln('   |+------------+|      ');
    writeln('   +-..--------..-+      ');
    writeln('   .--------------.      ');
    writeln('  / /============\ \     ');
    writeln(' / /==============\ \    ');
    writeln('/____________________\   ');
    writeln('\____________________/   ');
    TextColor(15);
    writeln;
    writeln('The computer starts to screech. A dead man switch?');
    writeln('Days Left: ', NuclearDestructionCountdown);
    writeln;
    writeln('R) Reset the Timer.');
    writeln('B) Log off computer.');
    RandomTitleBlock;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'R','r' :
                Begin
                  NuclearDestructionCountdown := 3;
                End;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure JournalRMS; { The Journal of RMS }

Begin
  Clrscr;
  RandomTitleBlock;
  writeln('FSF Avenger - RMS''s Journal');
  RandomTitleBlock;
  TextColor(5);
  writeln('                               +--------------+      ');
  writeln('                               |   \   \   \  |      ');
  writeln('                               |              |      ');
  writeln('                               | \   \   \    |      ');
  writeln('                               +--------------+      ');
  TextColor(15);
  writeln;
  writeln('The administrators wanted to force the users to have passwords. I''m not willing');
  writeln('to help uphold the security regime. They can''t tell me what to do! Those stupid');
  writeln('motherfuckers.');
  writeln;
  writeln('On another matter, I may have managed to track down the infamous Bryan Lunduke.');
  writeln('You have to summon him by activating three "magic" switches. It seems a bit');
  writeln('spooky to me...');
  writeln;
  writeln('HAHAHAHAHAHA! (Why did I write down laughing?)');
  writeln;
  writeln('Anyway, I''m afraid I wrote down the hint at the GNU Slash Bar. I went back for');
  writeln('it but apparently Eric Raymond stole it. If I can get it back, I''ll know where');
  writeln('to go next.');
  RandomTitleBlock;
  mashkey;
End;

Procedure OfficeRMS; { The office of RMS }

Var 
  c : char;
  Password: string;

Begin
  Repeat
    Clrscr;
    RandomTitleBlock;
    writeln('FSF Avenger - RMS''s Office');
    RandomTitleBlock;
    writeln('You are inside the Office of Richard M. Stallman.');
    writeln;
    TextColor(5);
    writeln('   +--------------+      ');
    writeln('   |.------------.|      ');
    writeln('   ||    FSF     ||      ');
    writeln('   ||            ||      ');
    writeln('   ||LOGIN: RMS  ||      ');
    writeln('   |+------------+|      ');
    writeln('   +-..--------..-+      ');
    writeln('   .--------------.      ');
    writeln('  / /============\ \     ');
    writeln(' / /==============\ \    ');
    writeln('/____________________\   ');
    writeln('\____________________/   ');
    TextColor(15);
    writeln;
    writeln('You notice a password prompt on the computer.');
    writeln;
    writeln('L) Log in to Computer.');
    writeln('R) Read RMS''s Journal.');
    writeln('B) Return to FSF HQ Lobby.');
    RandomTitleBlock;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'L','l' :
                Begin
                  write('Password> ');
                  readln(Password);
                  If (Password = '') And (BryanIsUnalived = false) Then
                    Begin
                      writeln;
                      writeln('The password is correct.');
                      If DoomsdaySwitch = false Then
                        Begin
                          DoomsdaySwitch := true;
                          NuclearDestructionCountdown := 3
                        End;
                      ArmageddonMachine;
                    End
                  Else
                    writeln('The password is incorrect.')
                End;
      'R','r' : JournalRMS;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure BuyDrugs; { The menu for buying drugs }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - The Non-free Repo - Buy Drugs');
    TitleBlock;
    writeln('You take a look at the selection of drugs for sale.');
    writeln;
    writeln('A whole galaxy of multi-coloured uppers, downers, laughers and screamers.');
    writeln('Enough to get you locked into a serious drug collection. You might push');
    writeln('it too far if you''re not careful.');
    writeln;
    writeln('V) Viagra (Bag: ', DrugViagra,')');
    writeln('N) Nitrous Oxide (Bag: ', DrugNitrous,')');
    writeln('C) Cocaine (Bag: ', DrugCocaine,')');
    writeln('L) LSD (Bag: ', DrugLSD,')');
    writeln('B) Return to Non-free Repo.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'V','v' :
                Begin
                  If Money >= 1000 Then
                    Begin
                      DrugViagra := DrugViagra+1;
                      Money := Money-1000;
                      writeln('You just got 1 viagra!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some money, honey!');
                      mashkey;
                    End
                End;
      'N','n' :
                Begin
                  If Money >= 1000 Then
                    Begin
                      DrugNitrous := DrugNitrous+1;
                      Money := Money-1000;
                      writeln('You just got 1 nitrous!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some money, honey!');
                      mashkey;
                    End
                End;
      'C','c' :
                Begin
                  If Money >= 1000 Then
                    Begin
                      DrugCocaine := DrugCocaine+1;
                      Money := Money-1000;
                      writeln('You just got 1 cocaine!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some money, honey!');
                      mashkey;
                    End
                End;
      'L','l' :
                Begin
                  If Money >= 1000 Then
                    Begin
                      DrugLSD := DrugLSD+1;
                      Money := Money-1000;
                      writeln('You just got 1 LSD!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some money, honey!');
                      mashkey;
                    End
                End;
      'B','b' : write;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure BannedBooks; { Various special moves that can be swapped out }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - The Non-free Repo - Illegal Books');
    TitleBlock;
    writeln('You take a look at the selection of books to borrow.');
    writeln;
    writeln('You can only borrow one book at a time.');
    writeln;
    writeln('A lot of sensitive topics you understand would be banned by the government.');
    writeln('Everything from bombs to illegal performance enhancements.');
    writeln;
    writeln('G) The Golden Book of Chemistry Experiments (Active: ', GoldenBook,')');
    writeln('T) Turbo Pascal 7.0 - 4th Edition (Active: ', TurboPascal,')');
    writeln('I) The IDA Pro Book (Active: ', IDAPro,')');
    writeln('B) Return to Non-free Repo.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'G','g' :
                Begin
                  GoldenBook := true;
                  IDAPro := false;
                  TurboPascal := false;
                  writeln;
                  writeln('You now have the Golden Book');
                  mashkey;
                End;
      'T','t' :
                Begin
                  GoldenBook := false;
                  IDAPro := false;
                  TurboPascal := true;
                  writeln;
                  writeln('You now have the Turbo Pascal book');
                  mashkey;
                End;
      'I','i' :
                Begin
                  GoldenBook := false;
                  IDAPro := true;
                  TurboPascal := false;
                  writeln;
                  writeln('You now have the IDA Pro book');
                  mashkey;
                End;
      'B','b' :
                Begin
                  write;
                End
                Else writeln('Wrong command');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure FirmwareUpgrades; { Permanent Buffs for the player }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - The Non-free Repo - Firmware Upgrades');
    TitleBlock;
    writeln('You take a look at the selection of firmware upgrades.');
    writeln;
    writeln('A lot of things that the FSF is against but you always have the freedom to');
    writeln('install these software patches on yourself.');
    writeln;
    writeln('E) Experience Booster - $5000 (Owned: ', ExperienceBooster,')');
    writeln('C) Cash Extractor - $2000 (Owned: ', CashExtractor,')');
    writeln('S) Special Move Extender - $5000 (Owned: ', SpecialExtender,')');
    writeln('B) Return to Non-free Repo.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'E','e' :
                Begin
                  If Money >= 5000 Then
                    Begin
                      ExperienceBooster := ExperienceBooster+1;
                      Money := Money-5000;
                      writeln('You just got 1 Experience Booster!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some money, honey!');
                      mashkey;
                    End
                End;
      'C','c' :
                Begin
                  If Money >= 2000 Then
                    Begin
                      CashExtractor := CashExtractor+1;
                      Money := Money-2000;
                      writeln('You just got 1 Cash Extrator!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some money, honey!');
                      mashkey;
                    End
                End;
      'S','s' :
                Begin
                  If Money >= 5000 Then
                    Begin
                      SpecialExtender := SpecialExtender+1;
                      SpecialsAmount := SpecialsAmount+1;
                      Money := Money-5000;
                      writeln('You just got 1 Special Move Extender!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some money, honey!');
                      mashkey;
                    End
                End;
      'B','b' : write;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure NonfreeRepo; { The main menu for the Non free repo }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - The Non-free Repo');
    TitleBlock;
    writeln('Just as you walk in you notice that things are not right.');
    writeln;
    writeln('You notice there is proprietary software everywhere.');
    writeln('You feel heat on your skin. It burns, but you will survive.');
    writeln;
    writeln('D) Buy Drugs');
    writeln('I) Illegal Books');
    writeln('F) Firmware Upgrades');
    writeln('B) Return to FSF HQ Lobby.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'D','d' : BuyDrugs;
      'I','i' : BannedBooks;
      'F','f' : FirmwareUpgrades;
      'B','b' : write('');
      Else writeln('Wrong command');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure SpikedDrinks; { The menu for spiking drinks }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - GNU Slash Bar - Bartender - Spike Drinks');
    TitleBlock;
    writeln('The bartender is preparing a number of drinks for other guests.');
    writeln;
    writeln('He keeps shaking and mixing. There is even a cool device that allows him to');
    writeln('measure a shot of alcohol quickly. Most people get two shots.');
    writeln;
    writeln('Now the bartender''s back is turned. Make your move...');
    writeln;
    writeln('V) Add Viagra to the Beer (Owned: ', DrugViagra,')');
    writeln('C) Put Cocaine in ESR''s Drink (Owned: ', DrugCocaine,')');
    writeln('L) LSD in the Fruit Punch (Owned: ', DrugLSD,')');
    writeln('B) Return to Bartender.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'V','v' :
                Begin
                  If DrugViagra >= 1 Then
                    Begin
                      DrugViagra := DrugViagra-1;
                      SpikedBeer := SpikedBeer+1;
                      Case SpikedBeer Of 
                        1:
                           Begin
                             writeln('A bunch of guys walk away from the bar with erections.');
                             mashkey;
                           End;
                        2:
                           Begin
                             writeln('More people are getting erections, this is funny!');
                             mashkey;
                           End;
                        3:
                           Begin
                             writeln(
                                'A serious amount of viagra is in the beer. More penises are erect.'
                             )
                             ;
                             mashkey;
                           End;
                        4:
                           Begin
                             writeln(
                                  'People are fucking wildly and screaming about being impregnated.'
                             );
                             mashkey;
                           End;
                        5:
                           Begin
                             writeln('Erections are everywhere');
                             writeln('Penises are now exploding. Blood and cum are everywhere.');
                             PenisesExploded := true;
                             mashkey;
                           End;
                        Else writeln(
                               'The bartender takes the viagra from you and tells you to "Fuck off"'
                          );
                        mashkey;
                      End;
                    End
                  Else
                    Begin
                      writeln('You don''t have enough viagra to share...');
                      mashkey;
                    End
                End;
      'C','c' :
                Begin
                  If DrugCocaine >= 1 Then
                    Begin
                      DrugCocaine := DrugCocaine-1;
                      CocaineCounter := CocaineCounter+1;
                      Case CocaineCounter Of 
                        1:
                           Begin
                             writeln('ESR doesn''t notice the cocaine you slipped into his drink.');
                             mashkey;
                           End;
                        2:
                           Begin
                             writeln('ESR is getting more agitated.');
                             mashkey;
                           End;
                        3:
                           Begin
                             writeln('Is Israel behind 9/11? ESR isn''t so sure...');
                             mashkey;
                           End;
                        4:
                           Begin
                             writeln('ESR finally catches you. He tells you "Keep it coming."');
                             mashkey;
                           End;
                        5:
                           Begin
                             writeln('A very angry ESR is now ranting about Arabs.');
                             mashkey;
                           End;
                        6:
                           Begin
                             writeln('Eric is now playing knife games on the counter of the bar.');
                             mashkey;
                           End;
                        7:
                           Begin
                             writeln(
                             'You comment to ESR that he is missing a finger. He blames socialists.'
                             );
                             mashkey;
                           End;
                        8:
                           Begin
                             writeln(
                                 'ESR is super mad. He is pounding his chest. He is jumping around.'
                             );
                             mashkey;
                           End;
                        9:
                           Begin
                             writeln('ESR is suffering from a heart attack. He is in the hospital.')
                             ;
                             IsESRaCorpse := true; { Magic Key }
                             mashkey;
                           End;
                        Else writeln(
                             'The bartender takes the cocaine from you and tells you to "Fuck off."'
                          );
                        mashkey;
                      End;
                    End
                  Else
                    Begin
                      writeln('You don''t have enough cocaine to spare...');
                      mashkey;
                    End
                End;
      'L','l' :
                Begin
                  If DrugLSD >= 1 Then
                    Begin
                      DrugLSD := DrugLSD-1;
                      SpikedPunch := SpikedPunch+1;
                      Case SpikedPunch Of 
                        1:
                           Begin
                             writeln('It takes a while but people start looking around with wonder.'
                             );
                             mashkey;
                           End;
                        2:
                           Begin
                             writeln(
                                    'People are starting to comment that someting odd is happening.'
                             );
                             mashkey;
                           End;
                        3:
                           Begin
                             writeln('Feces are being smeared on the wall. The King is coming.');
                             mashkey;
                           End;
                        4:
                           Begin
                             writeln('Some people are fucking. Others are tripping out.');
                             mashkey;
                           End;
                        5:
                           Begin
                             writeln('Everyone is crying or screaming. They are having a bad trip.')
                             ;
                             mashkey;
                           End;
                        Else writeln(
                                 'The bartender takes the LSD from you and tells you to "Fuck off."'
                          );
                        mashkey;
                      End;
                    End
                  Else
                    Begin
                      writeln('You don''t have enough LSD to share...');
                      mashkey;
                    End
                End;
      'B','b' : write;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure TalkToBartender; { Do various pranky things with the bartender }

Var 
  c : char;
  DrinkName : array[0..6] Of string;
  DrinkNameString : string;
  DayOfWeek: Word;
  Year, Month, Day: Word;


Begin
  Repeat
    GetDate(Year, Month, Day, DayOfWeek);
    DrinkName[0] := 'Liquid Cocaine.';
    DrinkName[1] := 'Blow Job.';
    DrinkName[2] := 'Martini, shaken, not stirred.';
    DrinkName[3] := 'Manhattan.';
    DrinkName[4] := 'Cosmopolitan.';
    DrinkName[5] := 'Straight up Bourbon.';
    DrinkName[6] := 'Wine, but it''s Dow''s 1927.';
    DrinkNameString := DrinkName[DayOfWeek];
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - GNU Slash Bar - Bartender');
    TitleBlock;
    writeln('You take a seat at the bar and decide to chat up the bartender.');
    writeln;
    writeln('People are ordering a variety of drinks.');
    writeln;
    writeln('O) Order the Bartender Special');
    writeln('S) Spike Drinks');
    writeln('B) Return to GNU Slash Bar.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'O','o' :
                Begin
                  If Money >= 50 Then
                    Begin
                      DrinksTaken := DrinksTaken+1;
                      If DrinksTaken >= 10 Then IsDrunk := true;
                      Money := Money-50;
                      writeln;
                      writeln('The bartender hands you a drink called ', DrinkNameString,
                              ' You drink it.'
                      );
                      writeln;
                      writeln(
                             'It tastes quite delicious. It has a bit of a kick to it, you love it.'
                      );
                      writeln;
                      writeln('He gives you a smirk. "That''s fifty bucks ya bum!"');
                      writeln;
                      writeln('The bartender takes $50 from you.');
                      writeln;
                      If IsDrunk = true Then writeln('You are totally wasted.');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some money, honey!');
                      mashkey;
                    End
                End;
      'S','s' : SpikedDrinks;
      'B','b' : write;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure FuckingPartyLetsGo; { The Menu for Let's Fucking Party }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - GNU Slash Bar - Let''s Fucking Party!');
    TitleBlock;
    writeln('You take a seat at the bar and decide to chat up the bartender.');
    writeln;
    writeln('People are ordering a variety of drinks.');
    writeln;
    writeln('T) Take a hit of Nitrous Oxide. (Cost 1) (Owned: ', DrugNitrous,')');
    writeln('G) Give everyone some Nitrous Oxide. (Cost 10) (Owned: ', DrugNitrous,')');
    If BryanIsUnalived = true Then
      Begin
        writeln('P) Party with Stallman');
      End;
    writeln('B) Return to GNU Slash Bar.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'T','t' :
                Begin
                  If DrugNitrous >= 1 Then
                    Begin
                      DrugNitrous := DrugNitrous-1;
                      Charisma := Charisma+1;
                      writeln;
                      writeln('You take a hit of nitrous oxide. You immediately burst out laughing.'
                      );
                      writeln;
                      writeln(
                         'Other people around you are also doing the stuff. Everyone is having fun.'
                      )
                      ;
                      writeln;
                      writeln('One of the dancing people come up to you. "This shit is the bomb!"');
                      writeln;
                      writeln('You both laugh hysterically at just about everything.');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some more nitrous, honey!');
                      mashkey;
                    End
                End;
      'G','g' :
                Begin
                  If DrugNitrous >= 10 Then
                    Begin
                      DrugNitrous := DrugNitrous-10;
                      Charisma := Charisma+10;
                      writeln;
                      writeln('You wheel a spare tire full of nitrous oxide over to the crowd.');
                      writeln;
                      writeln('"Free Nitrous Oxide now available in the spare tire" you say.');
                      writeln;
                      writeln('Everyone freaks out and starts taking hits!');
                      writeln;
                      writeln(
                           'Everyone is laughing hysterically. You can feel the love from everyone.'
                      );
                      writeln;
                      writeln('You are now at ', Charisma,' Charisma!');
                      mashkey;
                    End
                  Else
                    Begin
                      writeln('Get some more nitrous, honey!');
                      mashkey;
                    End
                End;
      'P','p' :
                Begin
                  writeln;
                  writeln('You and RMS wheel a spare tire full of nitrous oxide over to the crowd.')
                  ;
                  writeln;
                  writeln('Richard can''t even wait, he starts taking hits!');
                  writeln;
                  TextColor(12);
                  writeln('"Hahahahahahaha I love this shit!"');
                  TextColor(15);
                  writeln;
                  writeln('Everyone else is laughing, taking hits and having a good time..');
                  writeln;
                  Charisma := Charisma+50;
                  writeln('You are now at ', Charisma,' Charisma!');
                  mashkey;
                End;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure FlirtWithStaff; { Flirt with Staff Members }


{ Okay, so if you are not already familiar with Pascal, this is how it works.
  Inside of an IF statement you do not need semicolons, normally.
  If you are using "begin" and "end" inside an IF statement, THAT stuff DOES need it.

  Another thing to note. If you have else if and else, the semicolon goes last.
  Just study the code here and you'll see what I mean.

  Further, because I'm using IF statements inside of case statements, inside of
  "begin" and "end," it means that the semicolon goes at the end of case's "end"
  statement. }

Var 
  c : char;
  IsLoved, WantsToFuck : boolean;

Begin
  If Charisma >= 50 Then
    WantsToFuck := true
  Else If Charisma >= 10 Then
         IsLoved := true;
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - GNU Slash Bar - Flirt with Staff');
    TitleBlock;
    writeln('You look around and notice a few very attractive staff members.');
    writeln('A few of them have looked over at you and smiled. Your chances are good.');
    writeln;
    writeln('M) Flirt with a Waiter.');
    writeln('F) Flirt with a Waitress.');
    writeln('N) Flirt with the non-binary Server.');
    writeln('B) Return to GNU Slash Bar.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'M','m' :
                Begin
                  If WantsToFuck = true Then
                    Begin
                      writeln('You look over to wink at him but he surprises you.');
                      mashkey;
                      Clrscr;
                      writeln('The waiter is standing over you as you sit at the table.');
                      writeln('He unzips his pants. You see his massive erection.');
                      mashkey;
                      writeln;
                      writeln('You take his massive dick in your mouth.');
                      writeln('His balls slap against your chin as you deepthroat him.');
                      mashkey;
                      writeln;
                      writeln('He is about to blow his load!!!');
                      CumShotSound;
                      GotLaid := true;
                      mashkey;
                      writeln('Oh yes. He blew his hot load all over your face.');
                      writeln('You are a good slut!');
                      mashkey;
                    End
                  Else If IsLoved = true Then
                         Begin
                           writeln('You wink at him, he winks back. You blush a little.');
                           mashkey;
                         End
                  Else
                    Begin
                      writeln('You wink at him but he looks disgusted with you, grumbled about HR.')
                      ;
                      mashkey;
                    End
                End;
      'F','f' :
                Begin
                  If WantsToFuck = true Then
                    Begin
                      writeln('You look over to wink at her but she surprises you.');
                      mashkey;
                      Clrscr;
                      writeln('The waitress sits on your lap as you sit at the table.');
                      writeln('She strokes your hair and asks you to go into the other room.');
                      mashkey;
                      writeln;
                      writeln('You follow her into the staff room and she pulls out a giant dildo.')
                      ;
                      writeln('It''s part of a strap on! Oh wow! She bends you over the table.');
                      writeln;
                      mashkey;
                      writeln;
                      writeln('She fucks you with the strap-on while spanking your ass.');
                      writeln('It''s really hot, you can feel fluids running down your leg.');
                      writeln('Take it like a slut!');
                      CumShotSound;
                      GotLaid := true;
                      mashkey;
                      writeln('You are SUCH a good slut. You kiss each other.');
                    End
                  Else If IsLoved = true Then
                         Begin
                           writeln('You wink at her, she winks back. You blush a little.');
                           mashkey;
                         End

                  Else
                    Begin
                      writeln('You wink at her but she looks disgusted with you, grumbled about HR.'
                      );
                      mashkey;
                    End
                End;
      'N','n' :
                Begin
                  If WantsToFuck = true Then
                    Begin
                      writeln('You look over to wink at them but they surprises you.');
                      mashkey;
                      Clrscr;
                      writeln('The server grabs you by the arms and smiles at you.');
                      writeln('They stroke your hair and ask you to go into the other room.');
                      mashkey;
                      writeln;
                      writeln(
                          'You follow them into the staff room and they pulls out an e-stim device!'
                      );
                      writeln('They attach it to your nipples and your crotch.');
                      mashkey;
                      writeln;
                      writeln('Fingers trace across your crotch as they activate the device.');
                      writeln('The e-stim toy buzzes against your sensitive areas.');
                      writeln('They tease you with low pulses across your holes.');
                      mashkey;
                      writeln;
                      writeln('Waves of current flood your crotch relentlessly.');
                      writeln('You''re tremplig from the electrical stimulation.');
                      writeln('They dial it up, and you gasp at the intensity.');
                      mashkey;
                      writeln;
                      writeln('Your crotch convulses as the sensation overwhelms you.');
                      writeln('They hold it there as you come hard.');
                      writeln('You cry out as the e-stim takes you over the edge.');
                      CumShotSound;
                      GotLaid := true;
                      mashkey;
                      writeln('The device powers down as you collapse into them.');
                      writeln('They smile as you recover, still sensitive to every touch.');
                    End
                  Else If IsLoved = true Then
                         Begin
                           writeln('You wink at them, they wink back. You blush a little.');
                           mashkey;
                         End

                  Else
                    Begin
                      writeln(
                             'You wink at them but they look disgusted with you, grumbled about HR.'
                      );
                      mashkey;
                    End
                End;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure DaysEnd; { Basically this ends the day and flips switches. }
Begin
  DaysPlayed := DaysPlayed+1;
  SpikedBeer := 0;
  SpikedPunch := 0;
  DrinksTaken := 0;
  PlayerHealth := MaxPlayerHealth;
  If DoomsdaySwitch = true Then
    NuclearDestructionCountdown := NuclearDestructionCountdown-1;
  Clrscr;
  TitleBlock;
  writeln('FSF Avenger - Day Ended');
  TitleBlock;
  writeln('You are tired from all your hard work. You go to sleep.');
  writeln;
  If (NuclearDestructionCountdown <= 0) And (DoomsdaySwitch = true) Then
    Begin
      writeln('Zzzzzzz.... Wait.. Did you forget something?');
      mashkey;
      Armageddon;
    End
  Else
    Begin
      writeln('Zzzzzzz....');
      mashkey;
    End;
  If IsDrunk = true Then
    Begin
      IsDrunk := false;
      TextColor(14);
      writeln('You are now sober.');
      TextColor(15);
      mashkey;
    End;
  If PenisesExploded = true Then
    Begin
      PenisesExploded := false;
      TextColor(14);
      writeln('The penises stopped exploding.');
      TextColor(15);
    End;
  If (GotLaid = true) And (BryanIsUnalived = false) Then LoveLetter;
  If (IsESRaCorpse = true) And (BryanIsUnalived = false) Then ESRsWill;
End;

Procedure GetARoom; { Prepare to end the day. }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - GNU Slash Bar - Get a Room');
    TitleBlock;
    writeln('The staff hand you keys to your room.');
    writeln('A few of them have looked over at you and smiled. Your chances are good.');
    writeln;
    writeln('G) Go to Sleep.');
    writeln('B) Return to GNU Slash Bar');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'G','g' : DaysEnd;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') or (c='b');
End;

Procedure EricRants; { ESR rants about stuff }

Var 
  c : char;
  strings : array[1..27] Of string;
  randomIndex : integer;
  selectedString : string;

Begin
   if IsESRaCorpse = true then exit;
  { You are all puppets, on strings, an array of strings }
  { These are real quotes, some paraphrasing notwithstanding }
  strings[1] := 'Sure, my permit says ''Self-Defense'', but the real reason is mental health.';
  strings[2] := 'When I hear the words social responsibility, I want to reach for my gun.';
  strings[3] := 'Never trust a man who praises compassion while pointing a gun at you.';
  strings[4] := 'Why is it that my tinfoil hat never seems to quite fit right?';
  strings[5] := 'Idiotarian, n.: Anyone who isn''t a trigger-happy neocon/liberventionist.';
  strings[6] := 'Rapists just love unarmed women. And the politicians who disarm them.';
  strings[7] := 'The right to bear arms is not just a freedom; it''s the mother of all freedoms.';
  strings[8] := 'Hoplophobes fear their own forbidden urges to commit violence.';
  strings[9] := 'Everything you know is wrong.';
  strings[10] := 'They are not having children because they are neurotic, self-absorbed messes.';
  strings[11] := 'Think of this as a form of evolutionary pressure.';
  strings[12] := 'Closed-source advocates are enemies of liberty';
  strings[13] := 'They are enablers in vendor schemes to spy on you and imprison you.';
  strings[14] := 'Idiotarianism gives aid and comfort to terrorists.';
  strings[15] := 'The woke mind virus is a lethal pathogen.';
  strings[16] := 'The kind of charity you can force out of people spreads even nastier diseases.';
  strings[17] := 'Our society won''t be truly free until ''None of the Above'' is always an option.'
  ;
  strings[18] := 'Don''t let them disarm you!';
  strings[19] := 'Barack Obama''s victory was due to voter fraud.';
  strings[20] := 'They are not merely harmlessly misguided cultists; they are traitors.';
  strings[21] := 'The numbers are being massively cooked by socialists.';
  strings[22] := 'SJW dezinformatsiya tactics are being replicated by women-in-tech groups.';
  strings[23] := 'The KGB psyops are shooting their mind control beams at you.';
  strings[24] := 'This is a clash of civilizations driven by the failure of Islamic/Arab culture.';
  strings[25] := 'Do not be alone with any female, ever, at a technical conference.';
  strings[26] := 'They are trying to collect your scalp.';
  strings[27] := 'I''ve got some bullets slathered in pork fat.';

  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - GNU Slash Bar - ESR Rants');
    TitleBlock;
    writeln('You sit next to Eric S Raymond. He smells like cheap cigarettes ');
    writeln('and washed out dreams. He looks like he wants to stab someone.');
    writeln;
    writeln('A) Ask ESR about the state of the world.');
    writeln('B) Return to FSF HQ Lobby.');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'A','a' :
          Begin
            randomIndex := Random(27)+1;
            selectedString := strings[randomIndex];
            TextColor(13);
            writeln;
            writeln('ESR says:');
            writeln(selectedString);
            TextColor(15);
            mashkey;
          End;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') or (c='b');
End;


Procedure SlashBar; { A lobby of sorts, a place to chillax and have fun }

Var 
  c : char;

Begin
  Repeat
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - GNU Slash Bar');
    TitleBlock;
    writeln('You enter the GNU Slash Bar and are immediately greeted by several');
    writeln('neckbeards. You watch a greasy old man and woman fuck on a table as you');
    writeln('walk through the weirdos. You sit down at a vegan coffee table.');
    writeln;
    If IsESRaCorpse = true Then
      writeln('You notice a memorial to Eric S. Raymond. Rest in Peace, friend.')
    Else
      writeln('You take notice of Eric S. Raymond doing karaoke in a bathrobe. ');
    writeln;
    writeln('L) Let''s Fucking Party');
    writeln('T) Talk to the Bartender');
    writeln('G) Get a Room');
    writeln('V) View Your Stats');
    writeln('E) Listen to ESR Rant about Stuff');
    writeln('F) Flirt with staff');    { charisma system }
    writeln('B) Return to FSF HQ Lobby');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'L','l' : FuckingPartyLetsGo;
      'T','t' : TalkToBartender;
      'G','g' : GetARoom;
      'V','v' : FullStats;
      'E','e' : If IsESRaCorpse = false Then
                  Begin
                    EricRants;
                  End;
      'F','f' : FlirtWithStaff;
      'B','b' : writeln;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='B') Or (c='b');
End;

Procedure MainMenu; { The main menu }

Var 
  c : char;

Begin
  Repeat
    LevelCheck;
    Clrscr;
    TitleBlock;
    writeln('FSF Avenger - FSF HQ Lobby');
    TitleBlock;
    writeln('This is where it all began. You inhale deeply, it is the smell of freedom.');
    writeln;
    writeln('You look over and notice someone took a shit on the carpet.');
    writeln;
    writeln('F) Freedom Fighting         R) Recovery Node        ');
    writeln('B) GNU Slash Bar            N) The non-free Repo    ');
    writeln('O) RMS''s Office             Q) Quit and Touch Grass ');
    TitleBlock;
    MiniStats;
    write('Enter Command:>');
    readln(c);
    Case c Of 
      'F','f' : FreedomFighting;
      'R','r' : RecoveryNode;
      'B','b' : SlashBar;
      'N','n' : NonfreeRepo;
      'O','o' : OfficeRMS;
      'Q','q' : QuitGame;
      Else writeln('Wrong command, fuckface!');
      mashkey;
    End;
  Until (c='Q') Or (c='q');
End;

{ The main procedure starts here. Just starts up other procedures }

Begin
  TextBackground(Black);
  TextColor(White);
  clrscr;
  Randomize; { Seed me baby }
  Assignment;
  EnemyGenerator;
  TitleBlock;
  writeln('Welcome to FSF Avenger');
  TitleBlock;
  song;
  mashkey;
  Epileptic;
  setup;
  mainmenu;
End.
